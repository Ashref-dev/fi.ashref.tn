package util

import (
	"strings"
	"testing"
)

func TestRedactSecrets(t *testing.T) {
	input := "API_KEY=abc123\nsecret: topsecret\n-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----\neyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0In0.signature\nsk-abcdef1234567890abcdef"
	out := RedactSecrets(input)
	if out == input {
		t.Fatalf("expected redaction")
	}
	if strings.Contains(out, "abc123") {
		t.Fatalf("expected api key to be redacted")
	}
	if strings.Contains(out, "PRIVATE KEY") && strings.Contains(out, "abc") {
		t.Fatalf("expected private key to be redacted")
	}
	if strings.Contains(out, "eyJhbGci") {
		t.Fatalf("expected JWT to be redacted")
	}
	if strings.Contains(out, "sk-abcdef") {
		t.Fatalf("expected sk key to be redacted")
	}
}
