// multi.ai.inbox gateway agenti — MCP/API gateway con chiavi scoped per dominio.
// Decisione founder 19/10/2026: niente connettori 1:1 con registrar esterni;
// gli agenti gestiscono domini e zone (MX, A, TXT, CNAME) SOLO attraverso
// questo gateway, che diventa l'unico confine di autorizzazione per le
// scritture DNS (la chiave super-admin di mailcow.conf resta fuori dal
// perimetro degli agenti).
//
// Componenti:
//   - Registrar: interfaccia unica verso i dati dominio/zone (review
//     Architect P1-2). Due adattatori: MailcowRegistrar (via json_api.php
//     esistente) e GatewayStub (test/ambienti dry). I connettori registrar
//     esterni (write Namecheap, Infomaniak) NON esistono: quando slittano in,
//     implementeranno QUESTA interfaccia, non il gateway.
//   - Chiavi scoped: Bearer token per dominio (APIKeyScope), hash SHA-256
//     dentro un file cifrato AES-256-GCM con la stessa masterkey e lo stesso
//     pattern AAD dei cred di sessione (S2). La chiave super-admin non è
//     accettata dal gateway MCP.
//   - MCP JSON-RPC 2.0 su /api/gateway: domains_list, zone_get, zone_set
//     (dry_run=true → diff preview obbligatoria, poi apply), zone_undo.
//   - /api/gateway/keys: crea/elenca/revoca chiavi (bootstrap super-admin).
//   - Ogni mutazione di zona è in audit log; rate limit per chiave.
//
// Semantica degli errori: autenticazione/scope → HTTP status (401/403/429/409,
// fail-closed); errori di tool → JSON-RPC isError (HTTP 200), come l'endpoint
// /api/mcp esistente.
package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	gwRateWindow = time.Minute
	gwRateMax    = 60 // richieste MCP per chiave / finestra (sliding window)
	gwAdminMax   = 20 // chiamate admin sull'endpoint chiavi per IP / finestra
	gwMaxRecords = 100
	gwTTLMin     = 60
	gwTTLMax     = 86400
	gwMaxTXTLen  = 255 // singola stringa TXT (RFC 1035)
	gwKeysAAD    = "gateway-keys-v1"
	gwKeysFile   = "gateway_keys.json"
	gwBodyLimit  = 1 << 20
)

var (
	errKeyUnknown = errors.New("chiave sconosciuta")
	errKeyExpired = errors.New("chiave scaduta")
	errKeyRevoked = errors.New("chiave revocata")

	gwRecTypes = map[string]bool{"A": true, "MX": true, "TXT": true, "CNAME": true}
	// Nomi record relativi alla zona; ammettono l'underscore iniziale (_dmarc,
	// dkim._domainkey). Le ZONE restano validate da validDomain, che non li
	// ammette.
	gwRelNameRe  = regexp.MustCompile(`^_?[a-z0-9]([a-z0-9_-]{0,61}[a-z0-9])?(\._?[a-z0-9]([a-z0-9_-]{0,61}[a-z0-9])?)*$`)
	gwTxtValueRe = regexp.MustCompile(`^[ -~]{1,255}$`)
)

// ---------------------------------------------------------------------------
// Registrar: l'unico confine verso i dati dominio/zone
// ---------------------------------------------------------------------------

// DNSRecord è l'unità di scambio di ogni adattatore Registrar. Il name è
// relativo alla zona ("@" = apice, oppure label tipo "www" o "dkim._domainkey").
type DNSRecord struct {
	Name   string `json:"name"`
	Type   string `json:"type"` // A | MX | TXT | CNAME
	Value  string `json:"value"`
	MXPref int    `json:"mx_pref,omitempty"` // solo MX
	TTL    int    `json:"ttl"`
}

// Registrar è l'unico confine verso i dati dominio/zone (review Architect
// P1-2: interfaccia ListDomains/GetZone/SetRecords con diff/undo). Due
// adattatori: MailcowRegistrar e GatewayStub. NESSUN connettore 1:1 verso
// registrar esterni (decisione founder 19/10/2026): il write Namecheap/
// Infomaniak, quando slitta in, implementa questa interfaccia.
type Registrar interface {
	ListDomains() ([]string, error)
	GetZone(domain string) ([]DNSRecord, error)
	// SetRecords applica la sostituzione COMPLETA della zona del dominio.
	SetRecords(domain string, records []DNSRecord) error
}

var (
	_ Registrar = MailcowRegistrar{}
	_ Registrar = (*GatewayStub)(nil)
)

// zoneChange descrive una singola modifica della zona: è l'unità del diff
// preview restituito da zone_set con dry_run=true e dell'apply.
type zoneChange struct {
	Op     string     `json:"op"` // add | update | delete
	Name   string     `json:"name"`
	Type   string     `json:"type"`
	Before *DNSRecord `json:"before,omitempty"`
	After  *DNSRecord `json:"after,omitempty"`
}

// gwFQDN compone il name relativo con la zona ("@" / "" = apice).
func gwFQDN(name, domain string) string {
	if name == "" || name == "@" {
		return domain
	}
	return name + "." + domain
}

// normalizeRecord restituisce la forma canonica di un record (name/value
// minuscoli, target senza punto finale, mx_pref di default 10) o un errore di
// validazione: formato per tipo, TTL 60-86400, lunghezze DNS.
func normalizeRecord(domain string, r DNSRecord) (DNSRecord, error) {
	name := strings.ToLower(strings.TrimSpace(r.Name))
	typ := strings.ToUpper(strings.TrimSpace(r.Type))
	value := strings.TrimSpace(r.Value)
	if !gwRecTypes[typ] {
		return DNSRecord{}, fmt.Errorf("tipo record non supportato: %s (validi: A, MX, TXT, CNAME)", typ)
	}
	if name != "@" && name != "" && !gwRelNameRe.MatchString(name) {
		return DNSRecord{}, fmt.Errorf("nome record non valido: %s", name)
	}
	if typ == "CNAME" && (name == "@" || name == "") {
		// regola DNS: nessun CNAME sull'apice (coesiste con SOA/NS/MX)
		return DNSRecord{}, errors.New("CNAME non ammesso sull'apice della zona")
	}
	if fqdn := gwFQDN(name, domain); len(fqdn) > 253 {
		return DNSRecord{}, fmt.Errorf("nome record troppo lungo: %s", fqdn)
	}
	if r.TTL < gwTTLMin || r.TTL > gwTTLMax {
		return DNSRecord{}, fmt.Errorf("TTL fuori range (%d-%d s): %d", gwTTLMin, gwTTLMax, r.TTL)
	}
	switch typ {
	case "A":
		ip := net.ParseIP(value)
		if ip == nil || ip.To4() == nil || strings.Contains(value, ":") {
			return DNSRecord{}, fmt.Errorf("valore A non valido (richiesto IPv4): %s", value)
		}
		value = ip.String()
	case "MX", "CNAME":
		value = strings.ToLower(strings.TrimSuffix(value, "."))
		if !validDomain(value) {
			return DNSRecord{}, fmt.Errorf("valore %s non valido (richiesto hostname): %s", typ, value)
		}
	case "TXT":
		if !gwTxtValueRe.MatchString(value) {
			return DNSRecord{}, fmt.Errorf("valore TXT non valido (max %d caratteri stampabili, trovati %d)", gwMaxTXTLen, len(value))
		}
	}
	out := DNSRecord{Name: name, Type: typ, Value: value, TTL: r.TTL}
	if typ == "MX" {
		// mx_pref omesso (0 dopo il decode JSON) → default 10; null MX
		// (RFC 7505) non supportato.
		if r.MXPref < 0 || r.MXPref > 65535 {
			return DNSRecord{}, fmt.Errorf("mx_pref fuori range (0-65535): %d", r.MXPref)
		}
		if r.MXPref == 0 {
			out.MXPref = 10
		} else {
			out.MXPref = r.MXPref
		}
	}
	return out, nil
}

// validateRecords normalizza e valida l'intero set desiderato: max 100 record
// per operazione, identità (name|type|value) unica, CNAME esclusivo sul
// proprio nome. La lista vuota è ammessa: la sostituzione completa azzererebbe
// la zona, e il diff preview obbligatorio la mostra prima del commit.
func validateRecords(domain string, recs []DNSRecord) ([]DNSRecord, error) {
	if len(recs) > gwMaxRecords {
		return nil, fmt.Errorf("troppi record per operazione (max %d): %d", gwMaxRecords, len(recs))
	}
	out := make([]DNSRecord, 0, len(recs))
	seen := map[string]bool{}
	for _, r := range recs {
		n, err := normalizeRecord(domain, r)
		if err != nil {
			return nil, err
		}
		id := recKey(n)
		if seen[id] {
			return nil, fmt.Errorf("record duplicato: %s %s %s", n.Name, n.Type, n.Value)
		}
		seen[id] = true
		out = append(out, n)
	}
	cnames := map[string]bool{}
	for _, n := range out {
		if n.Type == "CNAME" {
			cnames[n.Name] = true
		}
	}
	for _, n := range out {
		if n.Type != "CNAME" && cnames[n.Name] {
			return nil, fmt.Errorf("il nome %s ha un CNAME: non puo' avere altri record", n.Name)
		}
	}
	return out, nil
}

// recKey è l'identità di un record dentro la zona: name|type|value.
// (Un nome può avere più A/MX con valori diversi; il CNAME è esclusivo.)
func recKey(r DNSRecord) string {
	v := r.Value
	if r.Type != "TXT" {
		v = strings.ToLower(v)
	}
	return r.Name + "|" + r.Type + "|" + v
}

// diffZone confronta la zona corrente con il set desiderato (sostituzione
// completa): add / update (ttl o mx_pref cambiati) / delete.
func diffZone(current, desired []DNSRecord) []zoneChange {
	cur := map[string]DNSRecord{}
	for _, r := range current {
		cur[recKey(r)] = r
	}
	des := map[string]DNSRecord{}
	for _, r := range desired {
		des[recKey(r)] = r
	}
	changes := []zoneChange{}
	for _, d := range desired {
		if c, ok := cur[recKey(d)]; !ok {
			after := d
			changes = append(changes, zoneChange{Op: "add", Name: d.Name, Type: d.Type, After: &after})
		} else if c.MXPref != d.MXPref || c.TTL != d.TTL {
			before, after := c, d
			changes = append(changes, zoneChange{Op: "update", Name: d.Name, Type: d.Type, Before: &before, After: &after})
		}
	}
	for _, c := range current {
		if _, ok := des[recKey(c)]; !ok {
			before := c
			changes = append(changes, zoneChange{Op: "delete", Name: c.Name, Type: c.Type, Before: &before})
		}
	}
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Name != changes[j].Name {
			return changes[i].Name < changes[j].Name
		}
		if changes[i].Type != changes[j].Type {
			return changes[i].Type < changes[j].Type
		}
		return changes[i].Op < changes[j].Op
	})
	return changes
}

func countOp(changes []zoneChange, op string) int {
	n := 0
	for _, c := range changes {
		if c.Op == op {
			n++
		}
	}
	return n
}

// sortedRecords + recordsHash: hash canonico (ordine-indipendente) del set
// desiderato, per verificare che l'apply combaci con l'ultimo dry_run.
func sortedRecords(recs []DNSRecord) []DNSRecord {
	out := append([]DNSRecord(nil), recs...)
	sort.Slice(out, func(i, j int) bool { return recKey(out[i]) < recKey(out[j]) })
	return out
}

func recordsHash(recs []DNSRecord) string {
	b, err := json.Marshal(sortedRecords(recs))
	if err != nil {
		return ""
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:8])
}

// ---------------------------------------------------------------------------
// Chiavi scoped (S2: hash SHA-256 + file cifrato AES-256-GCM, stessa masterkey)
// ---------------------------------------------------------------------------

// APIKeyScope è lo scope di una chiave gateway: un dominio (vuoto = tutti i
// domini, sola lettura), permesso di scrittura, scadenza (zero = senza scadenza).
type APIKeyScope struct {
	Domain    string    `json:"domain"`
	CanWrite  bool      `json:"can_write"`
	ExpiresAt time.Time `json:"expires_at"`
}

// gatewayKeyRecord: il token Bearer non è mai memorizzato in chiaro — solo
// l'hash SHA-256, dentro il file cifrato AES-256-GCM (stessa masterkey e
// pattern AAD dei cred di sessione; qui l'AAD lega il record allo scopo
// "chiavi gateway").
type gatewayKeyRecord struct {
	ID        string      `json:"id"`
	Hash      string      `json:"hash"`
	Scope     APIKeyScope `json:"scope"`
	CreatedAt time.Time   `json:"created_at"`
	Revoked   bool        `json:"revoked,omitempty"`
}

type gatewayKeyStore struct {
	mu   sync.Mutex
	path string
	keys []gatewayKeyRecord
}

func newGatewayKeyStore() *gatewayKeyStore {
	return &gatewayKeyStore{path: filepath.Join(credDir(), gwKeysFile)}
}

// load decifra il file chiavi; file assente = store vuoto (fail-closed:
// senza chiavi create nessun Bearer passa). Ricaricato a ogni operazione così
// che scadenze e revocche emesse da un altro processo valgano subito.
func (s *gatewayKeyStore) loadLocked() error {
	s.keys = nil
	ct, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	g := aead()
	if len(ct) < g.NonceSize() {
		return errors.New("file chiavi gateway troppo corto")
	}
	nonce, body := ct[:g.NonceSize()], ct[g.NonceSize():]
	pt, err := g.Open(nil, nonce, body, []byte(gwKeysAAD))
	if err != nil {
		return err
	}
	return json.Unmarshal(pt, &s.keys)
}

// saveLocked scrive con write atomico via rename (come storeCreds).
func (s *gatewayKeyStore) saveLocked() error {
	pt, err := json.Marshal(s.keys)
	if err != nil {
		return err
	}
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	ct := aead().Seal(nonce, nonce, pt, []byte(gwKeysAAD))
	if err := os.MkdirAll(credDir(), 0o700); err != nil {
		return err
	}
	tmp := filepath.Join(credDir(), "."+gwKeysFile+"."+newToken(6))
	if err := os.WriteFile(tmp, ct, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// validateScope: can_write=true richiede un dominio concreto (nessuna chiave
// di scrittura globale — least privilege, P0-1); il dominio, se presente, deve
// essere valido; expires_at, se presente, nel futuro.
func validateScope(sc APIKeyScope) error {
	sc.Domain = strings.ToLower(strings.TrimSpace(sc.Domain))
	if sc.Domain != "" && !validDomain(sc.Domain) {
		return errors.New("dominio scope non valido")
	}
	if sc.CanWrite && sc.Domain == "" {
		return errors.New("una chiave di scrittura richiede un dominio scoped (nessuna scrittura globale)")
	}
	if !sc.ExpiresAt.IsZero() && !sc.ExpiresAt.After(time.Now()) {
		return errors.New("expires_at deve essere nel futuro (o omesso)")
	}
	return nil
}

// Create emette una chiave scoped: il token è restituito UNA sola volta,
// nel file resta solo l'hash.
func (s *gatewayKeyStore) Create(sc APIKeyScope) (id, token string, err error) {
	if err := validateScope(sc); err != nil {
		return "", "", err
	}
	token = newToken(32) // 64 hex
	h := sha256.Sum256([]byte(token))
	id = "gwk_" + hex.EncodeToString(h[:4])
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return "", "", err
	}
	s.keys = append(s.keys, gatewayKeyRecord{
		ID:        id,
		Hash:      hex.EncodeToString(h[:]),
		Scope:     sc,
		CreatedAt: time.Now().UTC(),
	})
	if err := s.saveLocked(); err != nil {
		return "", "", err
	}
	return id, token, nil
}

// Verify risolve il Bearer token nello scope: confronto constant-time
// sull'hash per chiave; chiavi scadute e revocate sono rifiutate con lo
// stesso errore del chiamante (nessun oracolo tra i tre casi).
func (s *gatewayKeyStore) Verify(token string) (string, *APIKeyScope, error) {
	if token == "" {
		return "", nil, errKeyUnknown
	}
	h := sha256.Sum256([]byte(token))
	hash := hex.EncodeToString(h[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return "", nil, err
	}
	for i := range s.keys {
		k := &s.keys[i]
		if subtle.ConstantTimeCompare([]byte(hash), []byte(k.Hash)) != 1 {
			continue
		}
		if k.Revoked {
			return k.ID, nil, errKeyRevoked
		}
		if !k.Scope.ExpiresAt.IsZero() && time.Now().After(k.Scope.ExpiresAt) {
			return k.ID, nil, errKeyExpired
		}
		return k.ID, &s.keys[i].Scope, nil
	}
	return "", nil, errKeyUnknown
}

// Revoke marca la chiave come revocata per id (revoca immediata).
func (s *gatewayKeyStore) Revoke(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return false, err
	}
	found := false
	for i := range s.keys {
		if s.keys[i].ID == id {
			s.keys[i].Revoked = true
			found = true
		}
	}
	if !found {
		return false, nil
	}
	return true, s.saveLocked()
}

// List restituisce i metadati delle chiavi (mai hash o token).
func (s *gatewayKeyStore) List() ([]gatewayKeyRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return nil, err
	}
	out := make([]gatewayKeyRecord, len(s.keys))
	copy(out, s.keys)
	return out, nil
}

// ---------------------------------------------------------------------------
// gateway: lega Registrar, store chiavi, diff-registry e undo
// ---------------------------------------------------------------------------

// gwLimiter: sliding window (come ipLimiter) per chiave gateway / IP admin.
type gwLimiter struct {
	mu sync.Mutex
	m  map[string][]time.Time
}

func (l *gwLimiter) allow(key string, max int, window time.Duration) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	recent := l.m[key][:0]
	for _, t := range l.m[key] {
		if now.Sub(t) < window {
			recent = append(recent, t)
		}
	}
	recent = append(recent, now)
	l.m[key] = recent
	if len(l.m) > 4096 { // prune anti-crescita (chiavi/IP senza hit recenti)
		for k, ts := range l.m {
			if len(ts) == 0 || now.Sub(ts[len(ts)-1]) > window {
				delete(l.m, k)
			}
		}
	}
	return len(recent) <= max
}

type gateway struct {
	reg  Registrar
	keys *gatewayKeyStore
	lim  *gwLimiter

	diffMu sync.Mutex
	// lastDiff[kid|domain] = hash del set approvato con l'ultimo dry_run:
	// l'apply è accettato solo se combacia (diff preview obbligatoria).
	lastDiff map[string]string

	undoMu sync.Mutex
	// undo[domain] = zona precedente all'ultima apply riuscita (undo a un
	// livello, in memoria — la persistenza è lavoro futuro).
	undo map[string][]DNSRecord
}

func newGateway(reg Registrar) *gateway {
	return &gateway{
		reg:      reg,
		keys:     newGatewayKeyStore(),
		lim:      &gwLimiter{m: map[string][]time.Time{}},
		lastDiff: map[string]string{},
		undo:     map[string][]DNSRecord{},
	}
}

// registerGatewayRoutes è l'unica registrazione che main() riceve dal gateway.
func registerGatewayRoutes(mux *http.ServeMux, reg Registrar) {
	gw := newGateway(reg)
	mux.HandleFunc("/api/gateway", gw.handleGatewayMCP)
	mux.HandleFunc("/api/gateway/keys", gw.handleGatewayKeys)
}

// ---------------------------------------------------------------------------
// Adattatore mailcow (json_api.php via REST v1, header X-API-Key)
// ---------------------------------------------------------------------------

// gwMailcowCall è il client json_api.php del gateway: status code e body
// grezzo in errore (il mailcowCall esistente scambia un 502 HTML in null —
// review CTO P2-2; qui il gateway non eredita il difetto).
func gwMailcowCall(verb, path string, body any) (int, []byte, error) {
	key := os.Getenv("MAILCOW_API_KEY")
	if key == "" {
		return 0, nil, errors.New("MAILCOW_API_KEY non impostata sul server")
	}
	base := strings.TrimRight(env("MAILCOW_API_URL", "http://nginx"), "/")
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		reader = strings.NewReader(string(b))
	}
	req, err := http.NewRequest(verb, base+path, reader)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("X-API-Key", key)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: mcTotalTO}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("API mailcow non raggiungibile: %v", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return 0, nil, fmt.Errorf("risposta API mailcow illeggibile: %v", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, b, fmt.Errorf("API mailcow HTTP %d: %.200s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return resp.StatusCode, b, nil
}

// mcSuccess valida una risposta mailcow di add/edit/delete: array (o oggetto
// singolo) di {"type": "...", "msg": [...]}; type != success → errore con il
// motivo restituito da mailcow.
func mcSuccess(body []byte) error {
	var arr []map[string]any
	if err := json.Unmarshal(body, &arr); err != nil {
		var obj map[string]any
		if err2 := json.Unmarshal(body, &obj); err2 != nil {
			return fmt.Errorf("risposta mailcow non valida: %.200s", strings.TrimSpace(string(body)))
		}
		arr = []map[string]any{obj}
	}
	for _, o := range arr {
		if t, _ := o["type"].(string); t != "success" {
			return fmt.Errorf("mailcow ha rifiutato (%s): %v", t, o["msg"])
		}
	}
	return nil
}

type mcOverride struct {
	ID          any    `json:"id"`
	Hostname    string `json:"hostname"`
	Type        string `json:"type"`
	Destination string `json:"destination"`
	TTL         any    `json:"ttl"`
	Active      any    `json:"active"`
}

type mcDomain struct {
	DomainName  string `json:"domain_name"`
	Description string `json:"description"`
	MaxQuota    any    `json:"maxquota"`
	DefQuota    any    `json:"defquota"`
}

func gwActive(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x == "1" || strings.EqualFold(x, "true")
	case float64:
		return x != 0
	}
	return false
}

func gwInt(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(x))
		return n
	case int:
		return x
	}
	return 0
}

// mxEncode / mxDecode: mailcow dns/override non ha un campo priorità MX;
// convenzione dell'adattatore: destination = "<pref> <target>" per i record
// MX (formato presentazione DNS, es. "10 mail.example.com"). Da verificare
// sul mailcow live prima del go-live (vedi review-gateway.md).
func mxEncode(pref int, target string) string {
	return fmt.Sprintf("%d %s", pref, target)
}

func mxDecode(destination string) (int, string, bool) {
	s := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(destination), "."))
	prefS, target, ok := strings.Cut(s, " ")
	if !ok {
		return 0, "", false
	}
	pref, err := strconv.Atoi(strings.TrimSpace(prefS))
	if err != nil || pref < 0 || pref > 65535 {
		return 0, "", false
	}
	return pref, strings.ToLower(strings.TrimSpace(target)), true
}

// MailcowRegistrar implementa Registrar sull'API mailcow esistente
// (json_api.php, REST v1, X-API-Key = MAILCOW_API_KEY). La "zona" di un
// dominio è il set di DNS override attivi (dns/override = record serviti da
// unbound): è l'unica superficie DNS scrivibile che json_api.php espone.
type MailcowRegistrar struct{}

func (MailcowRegistrar) ListDomains() ([]string, error) {
	_, body, err := gwMailcowCall(http.MethodGet, "/api/v1/get/domain/all", nil)
	if err != nil {
		return nil, err
	}
	var domains []mcDomain
	if err := json.Unmarshal(body, &domains); err != nil {
		return nil, fmt.Errorf("risposta domini mailcow non valida: %v", err)
	}
	out := make([]string, 0, len(domains))
	for _, d := range domains {
		if d.DomainName != "" {
			out = append(out, strings.ToLower(strings.TrimSpace(d.DomainName)))
		}
	}
	return out, nil
}

// mcInDomainRecords mappa gli override attivi di un dominio in DNSRecord,
// con l'identità record → id override (serve per delete/update).
func mcInDomainRecords(domain string, overrides []mcOverride) ([]DNSRecord, map[string]int) {
	recs := []DNSRecord{}
	ids := map[string]int{}
	for _, o := range overrides {
		host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(o.Hostname), "."))
		if host != domain && !strings.HasSuffix(host, "."+domain) {
			continue
		}
		if !gwActive(o.Active) {
			continue
		}
		typ := strings.ToUpper(strings.TrimSpace(o.Type))
		rel := "@"
		if host != domain {
			rel = strings.TrimSuffix(host, "."+domain)
		}
		rec := DNSRecord{Name: rel, Type: typ, Value: strings.TrimSpace(o.Destination), TTL: gwInt(o.TTL)}
		if typ == "MX" {
			pref, target, ok := mxDecode(o.Destination)
			if !ok {
				pref, target = 10, strings.ToLower(rec.Value)
			}
			rec.Value, rec.MXPref = target, pref
		}
		ids[recKey(rec)] = gwInt(o.ID)
		recs = append(recs, rec)
	}
	return recs, ids
}

func (MailcowRegistrar) GetZone(domain string) ([]DNSRecord, error) {
	_, body, err := gwMailcowCall(http.MethodGet, "/api/v1/get/dns/override/all", nil)
	if err != nil {
		return nil, err
	}
	var overrides []mcOverride
	if err := json.Unmarshal(body, &overrides); err != nil {
		return nil, fmt.Errorf("risposta dns/override mailcow non valida: %v", err)
	}
	recs, _ := mcInDomainRecords(domain, overrides)
	return recs, nil
}

// SetRecords sostituisce la zona: delete degli override non più desiderati,
// add dei nuovi, update di quelli con TTL/priority cambiati. Operazioni
// sequenziali: un fallimento a metà lascia stato parziale — il re-run dello
// stesso set (dopo il nuovo dry_run obbligatorio) converge.
func (MailcowRegistrar) SetRecords(domain string, records []DNSRecord) error {
	_, body, err := gwMailcowCall(http.MethodGet, "/api/v1/get/dns/override/all", nil)
	if err != nil {
		return err
	}
	var overrides []mcOverride
	if err := json.Unmarshal(body, &overrides); err != nil {
		return fmt.Errorf("risposta dns/override mailcow non valida: %v", err)
	}
	curRecs, curIDs := mcInDomainRecords(domain, overrides)
	curByKey := map[string]DNSRecord{}
	for _, r := range curRecs {
		curByKey[recKey(r)] = r
	}
	desKeys := map[string]bool{}
	for _, r := range records {
		desKeys[recKey(r)] = true
	}
	for key, id := range curIDs {
		if desKeys[key] {
			continue
		}
		_, body, err := gwMailcowCall(http.MethodPost, "/api/v1/delete/dns/override", map[string]any{"id": []string{fmt.Sprint(id)}})
		if err == nil {
			err = mcSuccess(body)
		}
		if err != nil {
			return fmt.Errorf("eliminazione override %d (%s): %v", id, key, err)
		}
	}
	for _, r := range records {
		host := gwFQDN(r.Name, domain)
		destination := r.Value
		if r.Type == "MX" {
			destination = mxEncode(r.MXPref, r.Value)
		}
		if id, exists := curIDs[recKey(r)]; exists {
			cur := curByKey[recKey(r)]
			if cur.TTL == r.TTL && cur.MXPref == r.MXPref {
				continue
			}
			_, body, err := gwMailcowCall(http.MethodPost, "/api/v1/edit/dns/override", map[string]any{
				"items": []string{fmt.Sprint(id)},
				"attr":  map[string]any{"destination": destination, "ttl": r.TTL},
			})
			if err == nil {
				err = mcSuccess(body)
			}
			if err != nil {
				return fmt.Errorf("modifica record %s %s: %v", r.Type, host, err)
			}
			continue
		}
		_, body, err := gwMailcowCall(http.MethodPost, "/api/v1/add/dns/override", map[string]any{
			"hostname": host, "type": r.Type, "destination": destination, "ttl": r.TTL, "active": "1",
		})
		if err == nil {
			err = mcSuccess(body)
		}
		if err != nil {
			return fmt.Errorf("aggiunta record %s %s: %v", r.Type, host, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// GatewayStub: Registrar in memoria per i test (e ambienti dry)
// ---------------------------------------------------------------------------

// GatewayStub è un Registrar in memoria: sostituzione full-replace della zona,
// contatore mutazioni per gli assert dei test. NESSUN provider esterno.
type GatewayStub struct {
	mu        sync.Mutex
	Domains   []string
	Zones     map[string][]DNSRecord
	Mutations int
}

func NewGatewayStub(domains ...string) *GatewayStub {
	return &GatewayStub{
		Domains: append([]string(nil), domains...),
		Zones:   map[string][]DNSRecord{},
	}
}

func (s *GatewayStub) ListDomains() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.Domains...), nil
}

func (s *GatewayStub) GetZone(domain string) ([]DNSRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.Zones[strings.ToLower(domain)]
	out := make([]DNSRecord, len(cur)) // mai nil: "records": [] nei risultati
	copy(out, cur)
	return out, nil
}

func (s *GatewayStub) SetRecords(domain string, records []DNSRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Zones[strings.ToLower(domain)] = append([]DNSRecord(nil), records...)
	s.Mutations++
	return nil
}

// ---------------------------------------------------------------------------
// MCP JSON-RPC 2.0 (chiavi scoped — la super-admin NON è accettata qui)
// ---------------------------------------------------------------------------

// gwBearer estrae il token da "Authorization: Bearer <token>".
func gwBearer(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

// gwTools: schema MCP dei 4 tool del gateway (descrizioni in italiano).
func gwTools() []map[string]any {
	recItem := map[string]any{
		"type":     "object",
		"required": []string{"name", "type", "value", "ttl"},
		"properties": map[string]any{
			"name":    map[string]any{"type": "string", "description": "Nome relativo alla zona: @ per l'apice, oppure label (www, dkim._domainkey)"},
			"type":    map[string]any{"type": "string", "enum": []string{"A", "MX", "TXT", "CNAME"}},
			"value":   map[string]any{"type": "string", "description": "IPv4 (A), hostname (MX/CNAME) o testo (TXT, max 255 caratteri)"},
			"mx_pref": map[string]any{"type": "integer", "description": "Priorità MX 0-65535 (default 10 se omesso)"},
			"ttl":     map[string]any{"type": "integer", "description": "TTL in secondi, 60-86400"},
		},
		"additionalProperties": false,
	}
	domainSchema := map[string]any{
		"type": "object", "required": []string{"domain"},
		"properties": map[string]any{
			"domain": map[string]any{"type": "string", "description": "Dominio della zona, es. example.com"},
		},
		"additionalProperties": false,
	}
	return []map[string]any{
		{
			"name":        "domains_list",
			"description": "Elenca i domini visibili a questa chiave: se la chiave e' scoped a un dominio, solo quello.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
		},
		{
			"name":        "zone_get",
			"description": "Legge i record attuali della zona del dominio (MX, A, TXT, CNAME) gestiti dal gateway.",
			"inputSchema": domainSchema,
		},
		{
			"name":        "zone_set",
			"description": "Imposta la zona del dominio (sostituzione completa; MX/A/TXT/CNAME, max 100 record, TTL 60-86400). OBBLIGATORIO: prima chiamata con dry_run=true che restituisce il diff senza applicare nulla, poi la STESSA chiamata con dry_run=false applica. L'apply senza un dry_run identico precedente viene rifiutato (HTTP 409).",
			"inputSchema": map[string]any{
				"type": "object", "required": []string{"domain", "records", "dry_run"},
				"properties": map[string]any{
					"domain":  map[string]any{"type": "string", "description": "Dominio della zona, es. example.com"},
					"records": map[string]any{"type": "array", "items": recItem, "description": "Set completo desiderato dei record della zona"},
					"dry_run": map[string]any{"type": "boolean", "description": "true = solo diff preview, nessuna scrittura; false = applica"},
				},
				"additionalProperties": false,
			},
		},
		{
			"name":        "zone_undo",
			"description": "Annulla l'ultima mutazione applicata sulla zona del dominio (ripristina lo stato precedente all'ultimo zone_set riuscito).",
			"inputSchema": domainSchema,
		},
	}
}

// handleGatewayMCP: JSON-RPC 2.0 (MCP Streamable HTTP) con Bearer chiave
// scoped. Fail-closed: senza file chiavi o token valido nessun tool risponde.
// L'ordine è: metodo → auth → rate limit per chiave → parse → dispatch.
func (g *gateway) handleGatewayMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonErr(w, "POST richiesto, questo endpoint parla JSON-RPC 2.0 (MCP Streamable HTTP)", http.StatusMethodNotAllowed)
		return
	}
	token := gwBearer(r)
	if token == "" {
		jsonErr(w, `Autorizzazione richiesta: "Authorization: Bearer <chiave-scoped>"`, http.StatusUnauthorized)
		return
	}
	kid, scope, err := g.keys.Verify(token)
	if err != nil {
		auditLog("gateway_auth_failed", map[string]any{"ip": clientIP(r), "reason": err.Error()})
		jsonErr(w, "Chiave gateway non valida per questo accesso (sconosciuta, scaduta o revocata)", http.StatusForbidden)
		return
	}
	if !g.lim.allow(kid, gwRateMax, gwRateWindow) {
		jsonErr(w, "Rate limit superato per questa chiave, attendi un minuto", http.StatusTooManyRequests)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, gwBodyLimit))
	if err != nil {
		jsonErr(w, "Richiesta JSON-RPC 2.0 non valida", http.StatusBadRequest)
		return
	}
	var req rpcReq
	if err := json.Unmarshal(body, &req); err != nil || req.JSONRPC != "2.0" || req.Method == "" {
		jsonErr(w, "Richiesta JSON-RPC 2.0 non valida", http.StatusBadRequest)
		return
	}
	auditLog("gateway_request", map[string]any{"method": req.Method, "kid": kid, "ip": clientIP(r)})
	switch req.Method {
	case "initialize":
		writeJSON(w, http.StatusOK, rpcResult(req.ID, map[string]any{
			"protocolVersion": mcpProtoVer,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "multi-ai-inbox-gateway", "title": "multi.ai.inbox Gateway", "version": "1.0.0"},
			"instructions":    "Gateway agenti multi.ai.inbox: chiavi scoped per dominio. zone_set richiede PRIMA una chiamata con dry_run=true (restituisce il diff senza scrivere), poi la stessa chiamata con dry_run=false; zone_undo annulla l'ultima mutazione applicata.",
		}))
	case "ping":
		writeJSON(w, http.StatusOK, rpcResult(req.ID, map[string]any{}))
	case "tools/list":
		writeJSON(w, http.StatusOK, rpcResult(req.ID, map[string]any{"tools": gwTools()}))
	case "tools/call":
		g.handleToolCall(w, r, req, kid, scope)
	default:
		if strings.HasPrefix(req.Method, "notifications/") {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		writeJSON(w, http.StatusOK, rpcError(req.ID, -32601, "Metodo non trovato: "+req.Method))
	}
}

// handleToolCall: autorizzazione scope → HTTP 403; errori di tool → JSON-RPC
// isError (HTTP 200), come l'endpoint /api/mcp esistente.
func (g *gateway) handleToolCall(w http.ResponseWriter, r *http.Request, req rpcReq, kid string, scope *APIKeyScope) {
	name := str(req.Params["name"])
	args, _ := req.Params["arguments"].(map[string]any)
	domain := strings.ToLower(strings.TrimSpace(str(args["domain"])))
	if name == "domains_list" {
		// chiave scoped → solo il suo dominio, senza toccare il registrar
		if scope.Domain != "" {
			g.writeToolData(w, req.ID, map[string]any{"ok": true, "domains": []string{scope.Domain}, "scoped": true})
			return
		}
		names, err := g.reg.ListDomains()
		if err != nil {
			g.writeToolErr(w, req.ID, err.Error())
			return
		}
		g.writeToolData(w, req.ID, map[string]any{"ok": true, "domains": names})
		return
	}
	if domain == "" || !validDomain(domain) {
		g.writeToolErr(w, req.ID, "Nome dominio richiesto e valido (trovato: "+orDefault(domain, "vuoto")+")")
		return
	}
	if scope.Domain != "" && domain != scope.Domain {
		auditLog("gateway_scope_denied", map[string]any{"tool": name, "domain": domain, "kid": kid, "ip": clientIP(r)})
		jsonErr(w, "Chiave non valida per il dominio richiesto (scope: "+scope.Domain+")", http.StatusForbidden)
		return
	}
	switch name {
	case "zone_get":
		recs, err := g.reg.GetZone(domain)
		if err != nil {
			g.writeToolErr(w, req.ID, err.Error())
			return
		}
		g.writeToolData(w, req.ID, map[string]any{"ok": true, "domain": domain, "records": recs})
	case "zone_set":
		g.handleZoneSet(w, r, req, kid, scope, domain, args)
	case "zone_undo":
		g.handleZoneUndo(w, r, req, kid, scope, domain)
	default:
		g.writeToolErr(w, req.ID, "Tool sconosciuto: "+name)
	}
}

// handleZoneSet: validazione → dry_run=true = solo diff preview (e approvazione
// dell'hash del set), dry_run=false = apply, accettato SOLO se l'hash combacia
// con l'ultimo dry_run della chiave sul dominio. Il diff preview è obbligatorio
// prima di ogni commit (decisione founder 19/10/2026).
func (g *gateway) handleZoneSet(w http.ResponseWriter, r *http.Request, req rpcReq, kid string, scope *APIKeyScope, domain string, args map[string]any) {
	if !scope.CanWrite {
		auditLog("gateway_scope_denied", map[string]any{"tool": "zone_set", "domain": domain, "kid": kid, "ip": clientIP(r)})
		jsonErr(w, "Chiave in sola lettura: zone_set richiede can_write", http.StatusForbidden)
		return
	}
	raw, ok := args["records"].([]any)
	if !ok {
		raw = []any{} // lista vuota = azzeramento zona (visibile nel diff preview)
	}
	desired := make([]DNSRecord, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			g.writeToolErr(w, req.ID, "record non validi: ogni elemento deve essere un oggetto {name, type, value, ttl}")
			return
		}
		desired = append(desired, DNSRecord{
			Name:   str(m["name"]),
			Type:   str(m["type"]),
			Value:  str(m["value"]),
			MXPref: gwInt(m["mx_pref"]),
			TTL:    gwInt(m["ttl"]),
		})
	}
	norm, err := validateRecords(domain, desired)
	if err != nil {
		g.writeToolErr(w, req.ID, err.Error())
		return
	}
	dryRun := args["dry_run"] == true
	hash := recordsHash(norm)
	if dryRun {
		current, err := g.reg.GetZone(domain)
		if err != nil {
			g.writeToolErr(w, req.ID, err.Error())
			return
		}
		changes := diffZone(current, norm)
		g.diffMu.Lock()
		g.lastDiff[kid+"|"+domain] = hash
		g.diffMu.Unlock()
		g.writeToolData(w, req.ID, map[string]any{
			"ok":      true,
			"dry_run": true,
			"domain":  domain,
			"diff":    changes,
			"adds":    countOp(changes, "add"),
			"updates": countOp(changes, "update"),
			"deletes": countOp(changes, "delete"),
			"next":    "richiama zone_set con dry_run=false e lo stesso set di record per applicare",
		})
		return
	}
	// apply: consuma l'approvazione solo se l'hash combacia (un dry_run = un
	// apply; un apply fallito per set diverso NON consuma l'approvazione)
	g.diffMu.Lock()
	approved := g.lastDiff[kid+"|"+domain]
	if approved != "" && approved == hash {
		delete(g.lastDiff, kid+"|"+domain)
	} else {
		approved = ""
	}
	g.diffMu.Unlock()
	if approved == "" {
		jsonErr(w, "Diff preview obbligatoria: chiama prima zone_set con dry_run=true e lo stesso set di record, poi applica", http.StatusConflict)
		return
	}
	current, err := g.reg.GetZone(domain)
	if err != nil {
		g.writeToolErr(w, req.ID, err.Error())
		return
	}
	changes := diffZone(current, norm)
	if err := g.reg.SetRecords(domain, norm); err != nil {
		g.writeToolErr(w, req.ID, err.Error())
		return
	}
	g.undoMu.Lock()
	g.undo[domain] = current // stato precedente all'apply (undo a un livello)
	g.undoMu.Unlock()
	auditLog("gateway_zone_set", map[string]any{
		"domain":  domain,
		"kid":     kid,
		"ip":      clientIP(r),
		"ok":      true,
		"adds":    countOp(changes, "add"),
		"updates": countOp(changes, "update"),
		"deletes": countOp(changes, "delete"),
	})
	g.writeToolData(w, req.ID, map[string]any{"ok": true, "domain": domain, "applied": changes})
}

// handleZoneUndo: ripristina lo stato precedente all'ultima apply riuscita.
func (g *gateway) handleZoneUndo(w http.ResponseWriter, r *http.Request, req rpcReq, kid string, scope *APIKeyScope, domain string) {
	if !scope.CanWrite {
		auditLog("gateway_scope_denied", map[string]any{"tool": "zone_undo", "domain": domain, "kid": kid, "ip": clientIP(r)})
		jsonErr(w, "Chiave in sola lettura: zone_undo richiede can_write", http.StatusForbidden)
		return
	}
	g.undoMu.Lock()
	prev, ok := g.undo[domain]
	delete(g.undo, domain)
	g.undoMu.Unlock()
	if !ok {
		g.writeToolErr(w, req.ID, "Nessuna mutazione da annullare per "+domain)
		return
	}
	if err := g.reg.SetRecords(domain, prev); err != nil {
		g.writeToolErr(w, req.ID, err.Error())
		return
	}
	auditLog("gateway_zone_undo", map[string]any{"domain": domain, "kid": kid, "ip": clientIP(r), "ok": true, "records": len(prev)})
	g.writeToolData(w, req.ID, map[string]any{"ok": true, "domain": domain, "restored": len(prev)})
}

func (g *gateway) writeToolData(w http.ResponseWriter, id json.RawMessage, data map[string]any) {
	pretty, _ := json.MarshalIndent(data, "", "  ")
	writeJSON(w, http.StatusOK, rpcResult(id, map[string]any{
		"content": []map[string]any{{"type": "text", "text": string(pretty)}},
	}))
}

func (g *gateway) writeToolErr(w http.ResponseWriter, id json.RawMessage, msg string) {
	writeJSON(w, http.StatusOK, rpcResult(id, map[string]any{
		"content": []map[string]any{{"type": "text", "text": msg}}, "isError": true,
	}))
}

// ---------------------------------------------------------------------------
// /api/gateway/keys — ciclo di vita delle chiavi scoped
// ---------------------------------------------------------------------------

// handleGatewayKeys: crea/elenca/revoca chiavi scoped. Bootstrap: il Bearer
// qui è la chiave super-admin MAILCOW_API_KEY (l'unico segreto esistente) —
// le chiavi create la sostituiscono come canale degli agenti; il ritiro della
// super-admin è il passo successivo (review-gateway.md, "cosa resta da fare").
func (g *gateway) handleGatewayKeys(w http.ResponseWriter, r *http.Request) {
	if !g.lim.allow("admin|"+clientIP(r), gwAdminMax, gwRateWindow) {
		jsonErr(w, "Troppe richieste sull'endpoint chiavi, attendi un minuto", http.StatusTooManyRequests)
		return
	}
	expected := os.Getenv("MAILCOW_API_KEY")
	if expected == "" {
		jsonErr(w, "Endpoint chiavi non abilitato: MAILCOW_API_KEY non impostata sul server", http.StatusServiceUnavailable)
		return
	}
	if subtle.ConstantTimeCompare([]byte(expected), []byte(gwBearer(r))) != 1 {
		auditLog("gateway_keys_auth_failed", map[string]any{"ip": clientIP(r)})
		jsonErr(w, "Autorizzazione richiesta: Bearer MAILCOW_API_KEY (bootstrap chiavi scoped)", http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodPost:
		var body struct {
			Domain    string `json:"domain"`
			CanWrite  bool   `json:"can_write"`
			ExpiresAt string `json:"expires_at"` // RFC3339, opzionale
		}
		b, err := io.ReadAll(io.LimitReader(r.Body, gwBodyLimit))
		if err != nil || json.Unmarshal(b, &body) != nil {
			jsonErr(w, "Body JSON non valido: {domain, can_write, expires_at?}", http.StatusBadRequest)
			return
		}
		sc := APIKeyScope{Domain: strings.ToLower(strings.TrimSpace(body.Domain)), CanWrite: body.CanWrite}
		if body.ExpiresAt != "" {
			t, err := time.Parse(time.RFC3339, body.ExpiresAt)
			if err != nil {
				jsonErr(w, "expires_at non valido (richiesto RFC3339, es. 2026-12-31T23:59:59Z)", http.StatusBadRequest)
				return
			}
			sc.ExpiresAt = t
		}
		id, token, err := g.keys.Create(sc)
		if err != nil {
			jsonErr(w, err.Error(), http.StatusBadRequest)
			return
		}
		auditLog("gateway_key_created", map[string]any{"kid": id, "domain": sc.Domain, "can_write": sc.CanWrite, "ip": clientIP(r)})
		writeJSON(w, http.StatusCreated, map[string]any{
			"ok":     true,
			"key_id": id,
			"token":  token, // mostrato UNA sola volta
			"scope":  map[string]any{"domain": sc.Domain, "can_write": sc.CanWrite, "expires_at": sc.ExpiresAt},
		})
	case http.MethodGet:
		keys, err := g.keys.List()
		if err != nil {
			jsonErr(w, "Store chiavi illeggibile", http.StatusInternalServerError)
			return
		}
		out := make([]map[string]any, 0, len(keys))
		for _, k := range keys {
			out = append(out, map[string]any{
				"key_id":     k.ID,
				"domain":     k.Scope.Domain,
				"can_write":  k.Scope.CanWrite,
				"expires_at": k.Scope.ExpiresAt,
				"created_at": k.CreatedAt,
				"revoked":    k.Revoked,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "keys": out})
	case http.MethodDelete:
		var body struct {
			KeyID string `json:"key_id"`
		}
		b, err := io.ReadAll(io.LimitReader(r.Body, gwBodyLimit))
		if err != nil || json.Unmarshal(b, &body) != nil || body.KeyID == "" {
			jsonErr(w, "Body JSON non valido: {key_id}", http.StatusBadRequest)
			return
		}
		found, err := g.keys.Revoke(body.KeyID)
		if err != nil {
			jsonErr(w, "Store chiavi non scrivibile", http.StatusInternalServerError)
			return
		}
		if !found {
			jsonErr(w, "Chiave non trovata: "+body.KeyID, http.StatusNotFound)
			return
		}
		auditLog("gateway_key_revoked", map[string]any{"kid": body.KeyID, "ip": clientIP(r)})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "revoked": body.KeyID})
	default:
		jsonErr(w, "Metodi: POST (crea), GET (elenca), DELETE (revoca)", http.StatusMethodNotAllowed)
	}
}
