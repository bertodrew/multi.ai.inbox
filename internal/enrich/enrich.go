// Package enrich runs inbound messages through the Rust gatekeeper (prompt
// injection screening and reply extraction) over a JSON lines pipe.
package enrich

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"time"

	"github.com/bertodrew/multi.ai.inbox/internal/core"
)

type Enricher struct {
	Svc *core.Service
	Bin string // path to the gatekeeper binary
	Log *slog.Logger

	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner
}

type request struct {
	ID      string `json:"id"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
	HTML    string `json:"html"`
	DMARC   string `json:"dmarc"`
}

type response struct {
	ID            string   `json:"id"`
	Status        string   `json:"status"`
	Verdict       string   `json:"verdict"`
	Score         float64  `json:"score"`
	Reasons       []string `json:"reasons"`
	Model         string   `json:"model"`
	ExtractedText string   `json:"extracted_text"`
	Error         string   `json:"error"`
}

func (e *Enricher) Run(ctx context.Context) {
	defer e.Close()
	for {
		n, err := e.RunOnce(ctx)
		if err != nil {
			e.Log.Error("enrich batch failed", "err", err)
		}
		if n == 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}
}

func (e *Enricher) RunOnce(ctx context.Context) (int, error) {
	jobs, err := e.Svc.ClaimEnrich(ctx, 20)
	if err != nil {
		return 0, err
	}
	for _, j := range jobs {
		var auth struct {
			DMARC string `json:"dmarc"`
		}
		_ = json.Unmarshal(j.Auth, &auth)
		res, err := e.scan(request{ID: j.ID, Subject: j.Subject, Text: j.Text, HTML: j.HTML, DMARC: auth.DMARC})
		v := core.Verdict{Status: "unavailable", Verdict: "clear", Reasons: []string{}, Model: "none"}
		if err != nil {
			e.Log.Warn("gatekeeper unavailable", "message", j.ID, "err", err)
			v.Reasons = []string{"gatekeeper_unavailable"}
		} else {
			v = core.Verdict{Status: res.Status, Verdict: res.Verdict, Score: res.Score, Reasons: res.Reasons,
				Model: res.Model, ExtractedText: res.ExtractedText}
		}
		if err := e.Svc.SaveEnrichment(ctx, j, v); err != nil {
			return len(jobs), err
		}
	}
	return len(jobs), nil
}

func (e *Enricher) scan(req request) (*response, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cmd == nil {
		if err := e.start(); err != nil {
			return nil, err
		}
	}
	line, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if _, err := e.stdin.Write(append(line, '\n')); err != nil {
		e.stop()
		return nil, fmt.Errorf("write to gatekeeper: %w", err)
	}
	if !e.stdout.Scan() {
		e.stop()
		return nil, fmt.Errorf("gatekeeper exited: %v", e.stdout.Err())
	}
	var res response
	if err := json.Unmarshal(e.stdout.Bytes(), &res); err != nil {
		return nil, err
	}
	if res.Status != "complete" {
		return nil, fmt.Errorf("gatekeeper: %s", res.Error)
	}
	return &res, nil
}

func (e *Enricher) start() error {
	cmd := exec.Command(e.Bin)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start gatekeeper: %w", err)
	}
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64<<10), 32<<20)
	e.cmd, e.stdin, e.stdout = cmd, stdin, sc
	return nil
}

func (e *Enricher) stop() {
	if e.cmd == nil {
		return
	}
	_ = e.stdin.Close()
	_ = e.cmd.Process.Kill()
	_ = e.cmd.Wait()
	e.cmd = nil
}

func (e *Enricher) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stop()
}
