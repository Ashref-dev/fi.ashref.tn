package util

import "regexp"

var (
	keyValuePattern = regexp.MustCompile(`(?i)(api_key|apikey|secret|token|password|access_key|private_key)\s*[:=]\s*([^\s"']+)`)
	privateKeyBlock = regexp.MustCompile(`(?is)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`)
	jwtPattern      = regexp.MustCompile(`eyJ[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+\.?[a-zA-Z0-9_-]*`)
	skPattern       = regexp.MustCompile(`(?i)sk-[a-z0-9]{20,}`)
)

// RedactSecrets removes likely secrets from text.
func RedactSecrets(input string) string {
	out := keyValuePattern.ReplaceAllString(input, `$1=[REDACTED]`)
	out = privateKeyBlock.ReplaceAllString(out, "[REDACTED PRIVATE KEY]")
	out = jwtPattern.ReplaceAllString(out, "[REDACTED JWT]")
	out = skPattern.ReplaceAllString(out, "[REDACTED KEY]")
	return out
}
