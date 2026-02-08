package util

import (
	"strings"
)

// TruncateBytes trims a string to maxBytes if needed.
func TruncateBytes(input string, maxBytes int) (string, bool) {
	if maxBytes <= 0 || len(input) <= maxBytes {
		return input, false
	}
	return input[:maxBytes], true
}

// TruncateLinesAndBytes limits lines and total byte count.
func TruncateLinesAndBytes(lines []string, maxLines int, maxBytes int) (out []string, truncated bool, byteCount int) {
	if maxLines <= 0 && maxBytes <= 0 {
		return lines, false, len(strings.Join(lines, "\n"))
	}
	for _, line := range lines {
		if maxLines > 0 && len(out) >= maxLines {
			truncated = true
			break
		}
		lineBytes := len(line)
		sep := 0
		if len(out) > 0 {
			sep = 1
		}
		if maxBytes > 0 && byteCount+sep+lineBytes > maxBytes {
			truncated = true
			break
		}
		if sep == 1 {
			byteCount++
		}
		byteCount += lineBytes
		out = append(out, line)
	}
	return out, truncated, byteCount
}

// Preview returns a short preview of text by limiting lines and bytes.
func Preview(text string, maxLines int, maxBytes int) string {
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	trimmed, _, _ := TruncateLinesAndBytes(lines, maxLines, maxBytes)
	return strings.Join(trimmed, "\n")
}
