package core

import (
	"strings"
	"testing"
)

func TestRedactHeld(t *testing.T) {
	held := `{"message":{"id":"1","subject":"s","screening":"review","text":"secret","html":"<b>secret</b>","extracted_text":"secret","preview":"secret"},"thread":{"preview":"secret","subject":"s"}}`
	out := string(redactHeld([]byte(held)))
	if strings.Contains(out, "secret") || !strings.Contains(out, `"subject":"s"`) {
		t.Fatalf("held payload not redacted: %s", out)
	}
	for _, clean := range []string{
		`{"message":{"screening":"allowed","text":"body"}}`,
		`{"message":{"text":"body"}}`,
		`{"domain":{"text":"body"}}`,
		`not json`,
	} {
		if got := string(redactHeld([]byte(clean))); got != clean {
			t.Fatalf("redactHeld(%s) = %s", clean, got)
		}
	}
}
