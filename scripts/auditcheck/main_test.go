package main

import (
	"strings"
	"testing"
)

func TestAuditGate(t *testing.T) {
	clean := `{"config":{"protocol_version":"v1.0.0","scanner_name":"govulncheck"}}
{"SBOM":{"modules":[]}}
{"osv":{"id":"GO-example"}}`
	for _, tc := range []struct {
		name, input string
		reject      bool
	}{
		{"clean with advisory metadata", clean, false},
		{"finding despite scanner exit zero", clean + `{"finding":{"osv":"GO-2026-5932","trace":[{"module":"golang.org/x/crypto"}]}}`, true},
		{"empty finding", clean + `{"finding":{}}`, true},
		{"empty", "", true},
		{"truncated", clean + `{"finding":`, true},
		{"trailing garbage", clean + `garbage`, true},
		{"null", `null`, true},
		{"missing SBOM", `{"config":{"protocol_version":"v1.0.0","scanner_name":"govulncheck"}}`, true},
		{"unknown message", clean + `{"error":{}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := check(strings.NewReader(tc.input)); (err != nil) != tc.reject {
				t.Fatalf("error = %v; reject = %v", err, tc.reject)
			}
		})
	}
}
