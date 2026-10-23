// Package metrics exposes counters and gauges in the Prometheus text format
// (version 0.0.4) with the standard library only. Counters are atomic, so
// every role can increment them from any goroutine; gauges are read when
// /metrics is scraped. The handler is served on its own listen address
// (MAI_METRICS_ADDR), never through Caddy.
package metrics

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Counter is a monotonically increasing count.
type Counter struct{ v atomic.Uint64 }

func (c *Counter) Inc()         { c.v.Add(1) }
func (c *Counter) Add(n uint64) { c.v.Add(n) }
func (c *Counter) Value() uint64 {
	return c.v.Load()
}

// CounterVec is a counter family keyed by label values. Label values must
// come from a small fixed set (route classes, reasons), never from input.
type CounterVec struct {
	name, help string
	labels     []string
	mu         sync.RWMutex
	series     map[string]*series
}

type series struct {
	values []string
	c      Counter
}

// With returns the counter for the label values, in label order.
func (v *CounterVec) With(values ...string) *Counter {
	if len(values) != len(v.labels) {
		panic(fmt.Sprintf("metrics: %s wants %d label values, got %d", v.name, len(v.labels), len(values)))
	}
	key := strings.Join(values, "\xff")
	v.mu.RLock()
	s := v.series[key]
	v.mu.RUnlock()
	if s != nil {
		return &s.c
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if s = v.series[key]; s == nil {
		s = &series{values: append([]string(nil), values...)}
		v.series[key] = s
	}
	return &s.c
}

// Sample is one gauge value produced at scrape time.
type Sample struct {
	Name, Help string
	Labels     map[string]string
	Value      float64
	Counter    bool // TYPE counter instead of gauge
}

// Collector produces samples at scrape time (database backlogs). It gets a
// context bounded by the scrape timeout.
type Collector func(ctx context.Context) ([]Sample, error)

// Registry holds every family; Default is the process wide one.
type Registry struct {
	mu         sync.Mutex
	counters   []*CounterVec
	collectors []namedCollector
	// Timeout bounds each collector per scrape (default 5 seconds).
	Timeout time.Duration
	Log     *slog.Logger
}

type namedCollector struct {
	name string
	fn   Collector
}

func NewRegistry() *Registry { return &Registry{} }

// NewCounterVec registers a counter family. Names end in _total.
func (r *Registry) NewCounterVec(name, help string, labels ...string) *CounterVec {
	v := &CounterVec{name: name, help: help, labels: labels, series: map[string]*series{}}
	r.mu.Lock()
	r.counters = append(r.counters, v)
	r.mu.Unlock()
	return v
}

// NewCounter registers a counter without labels.
func (r *Registry) NewCounter(name, help string) *Counter {
	return r.NewCounterVec(name, help).With()
}

// Collect registers a scrape time collector. name prefixes its
// <name>_scrape_success gauge (1 when the last collection worked).
func (r *Registry) Collect(name string, fn Collector) {
	r.mu.Lock()
	r.collectors = append(r.collectors, namedCollector{name, fn})
	r.mu.Unlock()
}

type family struct {
	help, typ string
	lines     []string
}

// Write renders every family in the text exposition format, sorted by name.
func (r *Registry) Write(ctx context.Context, b *strings.Builder) {
	r.mu.Lock()
	counters := append([]*CounterVec(nil), r.counters...)
	collectors := append([]namedCollector(nil), r.collectors...)
	r.mu.Unlock()

	families := map[string]*family{}
	add := func(name, help, typ string, labels map[string]string, value float64) {
		f := families[name]
		if f == nil {
			f = &family{help: help, typ: typ}
			families[name] = f
		}
		f.lines = append(f.lines, name+formatLabels(labels)+" "+formatValue(value))
	}
	for _, v := range counters {
		v.mu.RLock()
		if len(v.series) == 0 && len(v.labels) > 0 {
			families[v.name] = &family{help: v.help, typ: "counter"} // HELP and TYPE before the first sample
		}
		for _, s := range v.series {
			labels := map[string]string{}
			for i, l := range v.labels {
				labels[l] = s.values[i]
			}
			add(v.name, v.help, "counter", labels, float64(s.c.Value()))
		}
		v.mu.RUnlock()
	}
	runtimeSamples(add)
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	for _, c := range collectors {
		cctx, cancel := context.WithTimeout(ctx, timeout)
		samples, err := c.fn(cctx)
		cancel()
		ok := 1.0
		if err != nil {
			ok = 0
			if r.Log != nil {
				r.Log.Warn("metrics collector failed", "collector", c.name, "err", err)
			}
		}
		for _, s := range samples {
			typ := "gauge"
			if s.Counter {
				typ = "counter"
			}
			add(s.Name, s.Help, typ, s.Labels, s.Value)
		}
		add(c.name+"_scrape_success", "1 when the "+c.name+" collector succeeded on this scrape.", "gauge", nil, ok)
	}

	names := make([]string, 0, len(families))
	for n := range families {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		f := families[n]
		b.WriteString("# HELP " + n + " " + escapeHelp(f.help) + "\n")
		b.WriteString("# TYPE " + n + " " + f.typ + "\n")
		sort.Strings(f.lines)
		for _, l := range f.lines {
			b.WriteString(l + "\n")
		}
	}
}

// Handler serves GET /metrics.
func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet && req.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var b strings.Builder
		r.Write(req.Context(), &b)
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(b.String()))
	})
}

func runtimeSamples(add func(name, help, typ string, labels map[string]string, value float64)) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	add("go_goroutines", "Number of goroutines that currently exist.", "gauge", nil, float64(runtime.NumGoroutine()))
	add("go_memstats_heap_alloc_bytes", "Bytes of allocated heap objects.", "gauge", nil, float64(ms.HeapAlloc))
	add("go_memstats_heap_inuse_bytes", "Bytes in in-use heap spans.", "gauge", nil, float64(ms.HeapInuse))
	add("go_memstats_sys_bytes", "Bytes of memory obtained from the OS.", "gauge", nil, float64(ms.Sys))
	add("go_gc_cycles_total", "Completed GC cycles.", "counter", nil, float64(ms.NumGC))
}

func formatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + `="` + escapeLabel(labels[k]) + `"`
	}
	return "{" + strings.Join(parts, ",") + "}"
}

var labelEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
var helpEscaper = strings.NewReplacer(`\`, `\\`, "\n", `\n`)

func escapeLabel(s string) string { return labelEscaper.Replace(s) }
func escapeHelp(s string) string  { return helpEscaper.Replace(s) }

func formatValue(v float64) string {
	switch {
	case math.IsInf(v, 1):
		return "+Inf"
	case math.IsInf(v, -1):
		return "-Inf"
	case math.IsNaN(v):
		return "NaN"
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}
