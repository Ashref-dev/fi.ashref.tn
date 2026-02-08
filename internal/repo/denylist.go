package repo

import (
	"path/filepath"
	"strings"
)

// IsDenylisted returns true if the file path should never be read.
func IsDenylisted(path string) bool {
	lower := strings.ToLower(path)
	base := strings.ToLower(filepath.Base(path))

	if strings.HasPrefix(base, ".env") {
		return true
	}
	if strings.HasSuffix(base, ".pem") || strings.HasSuffix(base, ".key") || strings.HasSuffix(base, ".p12") || strings.HasSuffix(base, ".pfx") {
		return true
	}
	if strings.HasPrefix(base, "id_rsa") {
		return true
	}
	if base == ".npmrc" {
		return true
	}
	if strings.Contains(lower, filepath.ToSlash(filepath.Join(".aws", "credentials"))) {
		return true
	}
	if strings.Contains(lower, filepath.ToSlash(filepath.Join(".docker", "config.json"))) {
		return true
	}
	return false
}
