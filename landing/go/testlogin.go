package main

// Test profile + mock Namecheap (21/10, founder request): so Playwright can
// exercise every authenticated function end-to-end, the container can boot a
// local Namecheap mock when LANDING_E2E_MODE=1. The site's login is NOT
// user+password — it is a Namecheap username + API key verified live against
// the Namecheap API; in e2e mode that verification runs against this mock.
//
// Fail-closed: the mock starts ONLY when LANDING_E2E_MODE=1 (never in
// production), accepts ONLY the e2e test credentials, and rejects everything
// else with a proper Namecheap error envelope — real credentials never pass
// through the mock.

import (
	"net/http"
	"os"
)

// e2e test credentials (test data, not real secrets; they match the connect
// regexes: username ^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$, key ^[A-Za-z0-9-]{8,128}$).
const (
	e2eUser = "testfounder"
	e2eKey  = "e2etest-key-0123456789abcdef"
)

func init() {
	if os.Getenv("LANDING_E2E_MODE") != "1" {
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/xml.response", e2eMockNC)
	// loopback only: nothing outside the container reaches the mock
	go func() { _ = http.ListenAndServe("127.0.0.1:18081", mux) }()
}

func e2eXMLEnvelope(inner string) string {
	return `<?xml version="1.0" encoding="utf-8"?>
<ApiResponse xmlns="http://api.namecheap.com/xml.response">` + inner + `</ApiResponse>`
}

// e2eDomainListXML: two test domains so load-domains, the gateway ownership
// guard and the settings PATCH path all have data to act on.
const e2eDomainListXML = `<?xml version="1.0" encoding="utf-8"?>
<ApiResponse xmlns="http://api.namecheap.com/xml.response">
  <Errors/>
  <RequestedCommand>namecheap.domains.getList</RequestedCommand>
  <CommandResponse Type="namecheap.domains.getList">
    <DomainGetListResult>
      <Domain ID="1" Name="example.org" Owner="testfounder" Created="2026-01-01" Expires="2027-01-01" IsExpired="false" IsLocked="false" AutoRenew="false" WhoisGuard="NOTPRESENT"/>
      <Domain ID="2" Name="example.net" Owner="testfounder" Created="2026-01-01" Expires="2027-01-01" IsExpired="false" IsLocked="false" AutoRenew="false" WhoisGuard="NOTPRESENT"/>
    </DomainGetListResult>
  </CommandResponse>
</ApiResponse>`

func e2eMockNC(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	// fail-closed even inside the mock: only the e2e test credentials pass
	if r.PostFormValue("ApiUser") != e2eUser || r.PostFormValue("ApiKey") != e2eKey {
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(e2eXMLEnvelope(`<Errors><Error Number="2011280">e2e mock: credentials rejected</Error></Errors>`)))
		return
	}
	w.Header().Set("Content-Type", "text/xml")
	_, _ = w.Write([]byte(e2eDomainListXML))
}
