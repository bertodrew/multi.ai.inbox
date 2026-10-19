package main

// Test del gateway agenti: validazione record, diff/undo, chiavi scoped
// (scaduta / dominio sbagliato / revocata / read-only → 403), rate limit per
// chiave, adattatore MailcowRegistrar su mock httptest (come main_test.go).
// Ogni test isola cred dir e masterkey via LANDING_CRED_DIR.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// gwEnv isola cred dir (→ masterkey + file chiavi) e MAILCOW_API_KEY per un test.
func gwEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LANDING_CRED_DIR", filepath.Join(t.TempDir(), "creds"))
	t.Setenv("MAILCOW_API_KEY", "test-super-admin-key")
}

func hashOf(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// gwRPC esegue una richiesta JSON-RPC diretta all'handler del gateway con
// Bearer opzionale e ritorna HTTP status + body parsato.
func gwRPC(t *testing.T, gw *gateway, token, method string, params map[string]any) (int, map[string]any) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	if err != nil {
		t.Fatalf("payload: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, "http://gw.test/api/gateway", strings.NewReader(string(payload)))
	if err != nil {
		t.Fatalf("request build: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	gw.handleGatewayMCP(rec, req)
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec.Code, body
}

// toolText estrae il primo content text da una risposta JSON-RPC.
func toolText(t *testing.T, body map[string]any) string {
	t.Helper()
	res, _ := body["result"].(map[string]any)
	if res == nil {
		t.Fatalf("risultato mancante: %v", body)
	}
	content, _ := res["content"].([]any)
	if len(content) == 0 {
		t.Fatalf("content mancante: %v", body)
	}
	first, _ := content[0].(map[string]any)
	txt, _ := first["text"].(string)
	return txt
}

// gwKeyViaHTTP crea una chiave scoped con l'endpoint admin (Bearer super-admin).
func gwKeyViaHTTP(t *testing.T, gw *gateway, method, payload, bearer string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(method, "http://gw.test/api/gateway/keys", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("request build: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	gw.handleGatewayKeys(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

// gwInjectKey inserisce un record chiave arbitrario (es. scaduto, che Create
// rifiuterebbe) nello store e lo persiste nel file cifrato.
func gwInjectKey(t *testing.T, gw *gateway, rec gatewayKeyRecord) {
	t.Helper()
	gw.keys.mu.Lock()
	defer gw.keys.mu.Unlock()
	gw.keys.keys = append(gw.keys.keys, rec)
	if err := gw.keys.saveLocked(); err != nil {
		t.Fatalf("persist keys: %v", err)
	}
}

// ---------------------------------------------------------------------------
// validazione input
// ---------------------------------------------------------------------------

func TestGatewayValidateRecords(t *testing.T) {
	gwEnv(t)
	cases := []struct {
		name    string
		rec     DNSRecord
		wantErr bool
	}{
		{"apex A", DNSRecord{Name: "@", Type: "A", Value: "10.0.0.1", TTL: 3600}, false},
		{"sub A", DNSRecord{Name: "www", Type: "A", Value: "10.0.0.2", TTL: gwTTLMin}, false},
		{"A con IPv6 rifiutato", DNSRecord{Name: "@", Type: "A", Value: "::1", TTL: 3600}, true},
		{"A con valore non IP", DNSRecord{Name: "@", Type: "A", Value: "mail.example.com", TTL: 3600}, true},
		{"MX con pref", DNSRecord{Name: "@", Type: "MX", Value: "mail.example.com", MXPref: 10, TTL: 3600}, false},
		{"MX target non hostname", DNSRecord{Name: "@", Type: "MX", Value: "10.0.0.1", MXPref: 10, TTL: 3600}, true},
		{"TXT", DNSRecord{Name: "@", Type: "TXT", Value: "v=spf1 mx -all", TTL: 3600}, false},
		{"TXT su _dmarc (underscore ammesso)", DNSRecord{Name: "_dmarc", Type: "TXT", Value: "v=DMARC1; p=none", TTL: 3600}, false},
		{"TXT troppo lungo", DNSRecord{Name: "@", Type: "TXT", Value: strings.Repeat("x", gwMaxTXTLen+1), TTL: 3600}, true},
		{"CNAME sub", DNSRecord{Name: "www", Type: "CNAME", Value: "target.example.com", TTL: 3600}, false},
		{"CNAME su apice", DNSRecord{Name: "@", Type: "CNAME", Value: "target.example.com", TTL: 3600}, true},
		{"tipo non valido", DNSRecord{Name: "@", Type: "NS", Value: "ns1.example.com", TTL: 3600}, true},
		{"TTL sotto il minimo", DNSRecord{Name: "@", Type: "A", Value: "10.0.0.1", TTL: gwTTLMin - 1}, true},
		{"TTL sopra il massimo", DNSRecord{Name: "@", Type: "A", Value: "10.0.0.1", TTL: gwTTLMax + 1}, true},
		{"nome non valido", DNSRecord{Name: "-bad-", Type: "A", Value: "10.0.0.1", TTL: 3600}, true},
		{"trailing dot normalizzato", DNSRecord{Name: "www", Type: "CNAME", Value: "target.example.com.", TTL: 3600}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := validateRecords("example.com", []DNSRecord{tc.rec})
			if (err != nil) != tc.wantErr {
				t.Fatalf("wantErr=%v, got %v", tc.wantErr, err)
			}
		})
	}
	// oltre 100 record per operazione
	many := make([]DNSRecord, gwMaxRecords+1)
	for i := range many {
		many[i] = DNSRecord{Name: fmt.Sprintf("r%d", i), Type: "A", Value: "10.0.0.1", TTL: 3600}
	}
	if _, err := validateRecords("example.com", many); err == nil {
		t.Fatalf("accettati %d record (max %d)", len(many), gwMaxRecords)
	}
	// CNAME esclusivo sullo stesso nome
	if _, err := validateRecords("example.com", []DNSRecord{
		{Name: "www", Type: "CNAME", Value: "t.example.com", TTL: 3600},
		{Name: "www", Type: "A", Value: "10.0.0.1", TTL: 3600},
	}); err == nil {
		t.Fatalf("CNAME + A sullo stesso nome accettati")
	}
	// duplicato esatto
	if _, err := validateRecords("example.com", []DNSRecord{
		{Name: "www", Type: "A", Value: "10.0.0.1", TTL: 3600},
		{Name: "www", Type: "A", Value: "10.0.0.1", TTL: 3600},
	}); err == nil {
		t.Fatalf("record duplicato accettato")
	}
}

func TestGatewayScopeValidation(t *testing.T) {
	gwEnv(t)
	if err := validateScope(APIKeyScope{Domain: "example.com", CanWrite: true, ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatalf("scope valido rifiutato: %v", err)
	}
	if err := validateScope(APIKeyScope{CanWrite: true}); err == nil {
		t.Fatalf("chiave di scrittura globale accettata (P0-1)")
	}
	if err := validateScope(APIKeyScope{Domain: "non e' un dominio"}); err == nil {
		t.Fatalf("dominio scope non valido accettato")
	}
	if err := validateScope(APIKeyScope{Domain: "example.com", ExpiresAt: time.Now().Add(-time.Hour)}); err == nil {
		t.Fatalf("expires_at nel passato accettato")
	}
	if err := validateScope(APIKeyScope{}); err != nil {
		t.Fatalf("chiave read-all rifiutata: %v", err)
	}
}

// ---------------------------------------------------------------------------
// diff / undo (review Architect P1-2)
// ---------------------------------------------------------------------------

func TestGatewayDiff(t *testing.T) {
	gwEnv(t)
	current := []DNSRecord{
		{Name: "@", Type: "A", Value: "10.0.0.1", TTL: 3600},
		{Name: "www", Type: "CNAME", Value: "old.example.com", TTL: 3600},
	}
	desired := []DNSRecord{
		{Name: "@", Type: "A", Value: "10.0.0.1", TTL: 3600},                // invariato
		{Name: "@", Type: "A", Value: "10.0.0.2", TTL: 3600},                // add
		{Name: "www", Type: "CNAME", Value: "old.example.com", TTL: 600},    // update (ttl)
		{Name: "_dmarc", Type: "TXT", Value: "v=DMARC1; p=none", TTL: 3600}, // add
	}
	changes := diffZone(current, desired)
	if countOp(changes, "add") != 2 || countOp(changes, "update") != 1 || countOp(changes, "delete") != 0 {
		t.Fatalf("diff errato: %+v", changes)
	}
	if ch := diffZone(current, current); len(ch) != 0 {
		t.Fatalf("set identico con modifiche: %+v", ch)
	}
	ch2 := diffZone(current, []DNSRecord{{Name: "x", Type: "A", Value: "10.9.9.9", TTL: 300}})
	if countOp(ch2, "delete") != 2 || countOp(ch2, "add") != 1 {
		t.Fatalf("replace errato: %+v", ch2)
	}
	// cambio mx_pref → update
	ch3 := diffZone(
		[]DNSRecord{{Name: "@", Type: "MX", Value: "mail.example.com", MXPref: 10, TTL: 3600}},
		[]DNSRecord{{Name: "@", Type: "MX", Value: "mail.example.com", MXPref: 20, TTL: 3600}},
	)
	if countOp(ch3, "update") != 1 {
		t.Fatalf("cambio mx_pref non rilevato: %+v", ch3)
	}
	// hash canonico ordine-indipendente (il dry_run e l'apply devono combaciare)
	a := []DNSRecord{
		{Name: "@", Type: "A", Value: "10.0.0.1", TTL: 3600},
		{Name: "www", Type: "CNAME", Value: "t.example.com", TTL: 300},
	}
	b := []DNSRecord{
		{Name: "www", Type: "CNAME", Value: "t.example.com", TTL: 300},
		{Name: "@", Type: "A", Value: "10.0.0.1", TTL: 3600},
	}
	if recordsHash(a) != recordsHash(b) {
		t.Fatalf("hash non ordine-indipendente")
	}
}

// ---------------------------------------------------------------------------
// flusso MCP completo su GatewayStub
// ---------------------------------------------------------------------------

func TestGatewayMCPFlow(t *testing.T) {
	gwEnv(t)
	stub := NewGatewayStub("example.com", "example.org")
	gw := newGateway(stub)

	// chiave scoped creata via endpoint admin (Bearer super-admin)
	code, out := gwKeyViaHTTP(t, gw, http.MethodPost, `{"domain":"example.com","can_write":true}`, os.Getenv("MAILCOW_API_KEY"))
	if code != http.StatusCreated {
		t.Fatalf("create key: %d %v", code, out)
	}
	token, _ := out["token"].(string)
	if token == "" {
		t.Fatalf("token mancante: %v", out)
	}
	if kid, _ := out["key_id"].(string); !strings.HasPrefix(kid, "gwk_") {
		t.Fatalf("key_id mancante: %v", out)
	}

	// initialize
	code, body := gwRPC(t, gw, token, "initialize", map[string]any{})
	res, _ := body["result"].(map[string]any)
	if code != http.StatusOK || res["protocolVersion"] != mcpProtoVer {
		t.Fatalf("initialize: %d %v", code, body)
	}

	// tools/list → 4 tool
	code, body = gwRPC(t, gw, token, "tools/list", map[string]any{})
	res, _ = body["result"].(map[string]any)
	tools, _ := res["tools"].([]any)
	if code != http.StatusOK || len(tools) != 4 {
		t.Fatalf("tools/list: %d, %d tool (attesi 4)", code, len(tools))
	}

	// domains_list con chiave scoped → solo example.com
	code, body = gwRPC(t, gw, token, "tools/call", map[string]any{"name": "domains_list", "arguments": map[string]any{}})
	txt := toolText(t, body)
	if !strings.Contains(txt, "example.com") || strings.Contains(txt, "example.org") {
		t.Fatalf("domains_list non filtrata dallo scope: %s", txt)
	}

	// zone_set dry_run=true → diff, NESSUNA mutazione
	recs := []map[string]any{{"name": "@", "type": "A", "value": "10.0.0.1", "ttl": 3600}}
	code, body = gwRPC(t, gw, token, "tools/call", map[string]any{"name": "zone_set", "arguments": map[string]any{"domain": "example.com", "records": recs, "dry_run": true}})
	if code != http.StatusOK {
		t.Fatalf("dry_run: %d %v", code, body)
	}
	txt = toolText(t, body)
	if !strings.Contains(txt, `"op": "add"`) {
		t.Fatalf("dry_run senza diff add: %s", txt)
	}
	if stub.Mutations != 0 {
		t.Fatalf("il dry_run ha mutato la zona (%d mutazioni)", stub.Mutations)
	}

	// apply con set DIVERSO dal dry_run → 409 (diff preview obbligatoria)
	other := []map[string]any{{"name": "@", "type": "A", "value": "10.0.0.9", "ttl": 3600}}
	code, _ = gwRPC(t, gw, token, "tools/call", map[string]any{"name": "zone_set", "arguments": map[string]any{"domain": "example.com", "records": other, "dry_run": false}})
	if code != http.StatusConflict {
		t.Fatalf("apply senza dry_run identico: atteso 409, got %d", code)
	}

	// apply identico → zona mutata
	code, _ = gwRPC(t, gw, token, "tools/call", map[string]any{"name": "zone_set", "arguments": map[string]any{"domain": "example.com", "records": recs, "dry_run": false}})
	if code != http.StatusOK {
		t.Fatalf("apply: %d", code)
	}
	if stub.Mutations != 1 {
		t.Fatalf("apply non registrato (%d mutazioni)", stub.Mutations)
	}
	zone, _ := stub.GetZone("example.com")
	if len(zone) != 1 || zone[0].Value != "10.0.0.1" {
		t.Fatalf("zona non applicata: %+v", zone)
	}

	// apply ripetuto senza nuovo dry_run → 409 (l'approvazione è consumata)
	code, _ = gwRPC(t, gw, token, "tools/call", map[string]any{"name": "zone_set", "arguments": map[string]any{"domain": "example.com", "records": recs, "dry_run": false}})
	if code != http.StatusConflict {
		t.Fatalf("apply ripetuto senza dry_run: atteso 409, got %d", code)
	}

	// zone_undo → ripristina lo stato precedente (zona vuota)
	code, body = gwRPC(t, gw, token, "tools/call", map[string]any{"name": "zone_undo", "arguments": map[string]any{"domain": "example.com"}})
	if code != http.StatusOK {
		t.Fatalf("undo: %d %v", code, body)
	}
	zone, _ = stub.GetZone("example.com")
	if len(zone) != 0 {
		t.Fatalf("undo non ha ripristinato: %+v", zone)
	}

	// zone_get legge la zona (ora vuota)
	code, body = gwRPC(t, gw, token, "tools/call", map[string]any{"name": "zone_get", "arguments": map[string]any{"domain": "example.com"}})
	if code != http.StatusOK {
		t.Fatalf("zone_get: %d %v", code, body)
	}
	txt = toolText(t, body)
	if !strings.Contains(txt, `"records": []`) {
		t.Fatalf("zone_get non legge la zona: %s", txt)
	}

	// validazione dentro il tool → JSON-RPC isError (HTTP 200)
	code, body = gwRPC(t, gw, token, "tools/call", map[string]any{"name": "zone_set", "arguments": map[string]any{
		"domain": "example.com", "dry_run": true,
		"records": []map[string]any{{"name": "@", "type": "BOGUS", "value": "x", "ttl": 3600}},
	}})
	if code != http.StatusOK {
		t.Fatalf("errore di validazione come HTTP %d (atteso 200 isError)", code)
	}
	res, _ = body["result"].(map[string]any)
	if isError, _ := res["isError"].(bool); !isError {
		t.Fatalf("errore di validazione senza isError: %v", body)
	}
}

// ---------------------------------------------------------------------------
// chiavi scoped: scaduta / dominio sbagliato / read-only / revocata / rate limit
// ---------------------------------------------------------------------------

func TestGatewayScopedKeys(t *testing.T) {
	gwEnv(t)
	stub := NewGatewayStub("good.test")
	gw := newGateway(stub)

	// scaduta → 403 (record iniettato direttamente: Create rifiuta scadenze passate)
	gwInjectKey(t, gw, gatewayKeyRecord{
		ID:    "gwk_expired",
		Hash:  hashOf("expired-token"),
		Scope: APIKeyScope{Domain: "good.test", CanWrite: true, ExpiresAt: time.Now().Add(-time.Hour)},
	})
	code, _ := gwRPC(t, gw, "expired-token", "tools/list", map[string]any{})
	if code != http.StatusForbidden {
		t.Fatalf("chiave scaduta: atteso 403, got %d", code)
	}

	// dominio sbagliato → 403
	_, token, err := gw.keys.Create(APIKeyScope{Domain: "good.test", CanWrite: true})
	if err != nil {
		t.Fatalf("create scoped: %v", err)
	}
	code, _ = gwRPC(t, gw, token, "tools/call", map[string]any{"name": "zone_get", "arguments": map[string]any{"domain": "other.test"}})
	if code != http.StatusForbidden {
		t.Fatalf("dominio sbagliato: atteso 403, got %d", code)
	}
	// sul dominio giusto passa
	if code, _ = gwRPC(t, gw, token, "tools/call", map[string]any{"name": "zone_get", "arguments": map[string]any{"domain": "good.test"}}); code != http.StatusOK {
		t.Fatalf("zone_get sul dominio scoped: %d", code)
	}

	// chiave read-only con zone_set → 403
	_, ro, err := gw.keys.Create(APIKeyScope{Domain: "good.test", CanWrite: false})
	if err != nil {
		t.Fatalf("create ro: %v", err)
	}
	code, _ = gwRPC(t, gw, ro, "tools/call", map[string]any{"name": "zone_set", "arguments": map[string]any{
		"domain": "good.test", "dry_run": true,
		"records": []map[string]any{{"name": "@", "type": "A", "value": "10.0.0.1", "ttl": 3600}},
	}})
	if code != http.StatusForbidden {
		t.Fatalf("chiave read-only con zone_set: atteso 403, got %d", code)
	}

	// token sconosciuto → 403; header assente → 401
	code, _ = gwRPC(t, gw, "no-such-token", "tools/list", map[string]any{})
	if code != http.StatusForbidden {
		t.Fatalf("token sconosciuto: atteso 403, got %d", code)
	}
	code, _ = gwRPC(t, gw, "", "tools/list", map[string]any{})
	if code != http.StatusUnauthorized {
		t.Fatalf("header assente: atteso 401, got %d", code)
	}

	// revocata → 403
	code, out := gwKeyViaHTTP(t, gw, http.MethodPost, `{"domain":"good.test","can_write":true}`, os.Getenv("MAILCOW_API_KEY"))
	if code != http.StatusCreated {
		t.Fatalf("create: %d %v", code, out)
	}
	kid, _ := out["key_id"].(string)
	revToken, _ := out["token"].(string)
	if code, _ = gwRPC(t, gw, revToken, "tools/list", map[string]any{}); code != http.StatusOK {
		t.Fatalf("chiave appena creata rifiutata: %d", code)
	}
	code, _ = gwKeyViaHTTP(t, gw, http.MethodDelete, `{"key_id":"`+kid+`"}`, os.Getenv("MAILCOW_API_KEY"))
	if code != http.StatusOK {
		t.Fatalf("revoke: %d", code)
	}
	if code, _ = gwRPC(t, gw, revToken, "tools/list", map[string]any{}); code != http.StatusForbidden {
		t.Fatalf("chiave revocata: atteso 403, got %d", code)
	}

	// endpoint chiavi: admin sbagliato → 403; write key senza dominio → 400
	code, _ = gwKeyViaHTTP(t, gw, http.MethodPost, `{"domain":"good.test"}`, "wrong-admin-key")
	if code != http.StatusForbidden {
		t.Fatalf("admin sbagliato: atteso 403, got %d", code)
	}
	code, _ = gwKeyViaHTTP(t, gw, http.MethodPost, `{"can_write":true}`, os.Getenv("MAILCOW_API_KEY"))
	if code != http.StatusBadRequest {
		t.Fatalf("write key senza dominio: atteso 400, got %d", code)
	}

	// rate limit per chiave: oltre gwRateMax richieste → 429
	_, rl, err := gw.keys.Create(APIKeyScope{Domain: "good.test", CanWrite: false})
	if err != nil {
		t.Fatalf("create rl: %v", err)
	}
	last := 0
	for i := 0; i <= gwRateMax; i++ {
		last, _ = gwRPC(t, gw, rl, "tools/list", map[string]any{})
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("rate limit per chiave: atteso 429 dopo %d richieste, got %d", gwRateMax+1, last)
	}
}

// ---------------------------------------------------------------------------
// store chiavi cifrato (stessa masterkey + AAD dei cred di sessione)
// ---------------------------------------------------------------------------

func TestGatewayKeysFileEncrypted(t *testing.T) {
	gwEnv(t)
	gw := newGateway(&GatewayStub{})
	_, token, err := gw.keys.Create(APIKeyScope{Domain: "example.com", CanWrite: true})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(os.Getenv("LANDING_CRED_DIR"), gwKeysFile))
	if err != nil {
		t.Fatalf("file chiavi assente: %v", err)
	}
	// il file NON contiene né il token né lo scope in chiaro
	if strings.Contains(string(raw), token) || strings.Contains(string(raw), "example.com") {
		t.Fatalf("file chiavi leggibile in chiaro: %q", raw[:60])
	}
	// roundtrip: un nuovo store sullo stesso file risolve la chiave
	gw2 := newGateway(&GatewayStub{})
	kid, scope, err := gw2.keys.Verify(token)
	if err != nil || kid == "" || scope == nil || scope.Domain != "example.com" || !scope.CanWrite {
		t.Fatalf("roundtrip chiavi cifrate: kid=%q scope=%v err=%v", kid, scope, err)
	}
}

// ---------------------------------------------------------------------------
// adattatore MailcowRegistrar su mock json_api.php (httptest, come main_test.go)
// ---------------------------------------------------------------------------

func TestMailcowRegistrar(t *testing.T) {
	gwEnv(t)
	var mu sync.Mutex
	var posts []map[string]any // richieste di mutazione viste dal mock
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/get/domain/all", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"domain_name":"example.com","active":"1"},{"domain_name":"example.org","active":"1"}]`))
	})
	mux.HandleFunc("/api/v1/get/dns/override/all", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"id":1,"hostname":"mail.example.com","type":"MX","destination":"10 mail.example.com","ttl":3600,"active":"1"},
			{"id":2,"hostname":"www.example.com","type":"CNAME","destination":"example.com","ttl":1800,"active":"1"},
			{"id":3,"hostname":"other.example","type":"A","destination":"10.5.5.5","ttl":300,"active":"1"},
			{"id":4,"hostname":"old.example.com","type":"A","destination":"10.9.9.9","ttl":300,"active":"0"}
		]`))
	})
	mut := func(name string, w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		mu.Lock()
		posts = append(posts, map[string]any{"op": name, "body": b})
		mu.Unlock()
		_, _ = w.Write([]byte(`[{"type":"success","msg":["ok"]}]`))
	}
	mux.HandleFunc("/api/v1/add/dns/override", func(w http.ResponseWriter, r *http.Request) { mut("add", w, r) })
	mux.HandleFunc("/api/v1/delete/dns/override", func(w http.ResponseWriter, r *http.Request) { mut("delete", w, r) })
	mux.HandleFunc("/api/v1/edit/dns/override", func(w http.ResponseWriter, r *http.Request) { mut("edit", w, r) })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	t.Setenv("MAILCOW_API_URL", srv.URL)

	reg := MailcowRegistrar{}

	domains, err := reg.ListDomains()
	if err != nil || len(domains) != 2 || domains[0] != "example.com" {
		t.Fatalf("ListDomains: %v %v", domains, err)
	}

	zone, err := reg.GetZone("example.com")
	if err != nil {
		t.Fatalf("GetZone: %v", err)
	}
	// solo gli override attivi nel dominio: MX + CNAME (esclusi other.example e l'inattivo)
	if len(zone) != 2 {
		t.Fatalf("GetZone: %+v", zone)
	}
	var mx *DNSRecord
	for i := range zone {
		if zone[i].Type == "MX" {
			mx = &zone[i]
		}
	}
	if mx == nil || mx.Name != "mail" || mx.MXPref != 10 || mx.Value != "mail.example.com" {
		t.Fatalf("MX decodificato male: %+v", mx)
	}

	// SetRecords: il set corrente (MX su "mail", CNAME su "www") copre MX e
	// CNAME → solo l'A nuovo è un add
	desired := []DNSRecord{
		{Name: "mail", Type: "MX", Value: "mail.example.com", MXPref: 10, TTL: 3600},
		{Name: "www", Type: "CNAME", Value: "example.com", TTL: 1800},
		{Name: "@", Type: "A", Value: "10.0.0.1", TTL: 3600},
	}
	if err := reg.SetRecords("example.com", desired); err != nil {
		t.Fatalf("SetRecords: %v", err)
	}
	mu.Lock()
	adds := 0
	var added map[string]any
	for _, p := range posts {
		if p["op"] == "add" {
			adds++
			added, _ = p["body"].(map[string]any)
		}
	}
	mu.Unlock()
	if adds != 1 {
		t.Fatalf("add attese 1, viste %d: %+v", adds, posts)
	}
	if got, _ := added["hostname"].(string); got != "example.com" {
		t.Fatalf("add con hostname errato: %v", added)
	}
	if got, _ := added["destination"].(string); got != "10.0.0.1" {
		t.Fatalf("add con destination errato: %v", added)
	}

	// rifiuto mailcow → errore propagato
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/get/") {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		_, _ = w.Write([]byte(`[{"type":"danger","msg":["quota_exceeded"]}]`))
	}))
	defer failing.Close()
	t.Setenv("MAILCOW_API_URL", failing.URL)
	if err := reg.SetRecords("example.com", []DNSRecord{{Name: "@", Type: "A", Value: "10.0.0.1", TTL: 3600}}); err == nil {
		t.Fatalf("rifiuto mailcow non propagato")
	}

	// HTTP 502 → errore (il gateway non restituisce null, review CTO P2-2)
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}))
	defer broken.Close()
	t.Setenv("MAILCOW_API_URL", broken.URL)
	if _, err := reg.ListDomains(); err == nil {
		t.Fatalf("HTTP 502 non propagato come errore")
	}
}
