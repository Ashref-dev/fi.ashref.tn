package util

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// LoadShellHistory returns the last N commands from shell history.
func LoadShellHistory(maxLines int) []string {
	if maxLines <= 0 {
		return nil
	}
	path := historyPath()
	if path == "" {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	if tail := readHistoryTail(file, maxLines); tail != nil {
		for i, line := range tail {
			tail[i] = RedactSecrets(line)
		}
		return tail
	}

	scanner := bufio.NewScanner(file)
	lines := make([]string, 0, maxLines)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lines = append(lines, normalizeHistoryLine(line))
		if len(lines) > maxLines {
			lines = lines[len(lines)-maxLines:]
		}
	}

	for i, line := range lines {
		lines[i] = RedactSecrets(line)
	}
	return lines
}

func readHistoryTail(file *os.File, maxLines int) []string {
	info, err := file.Stat()
	if err != nil {
		return nil
	}
	size := info.Size()
	if size <= 0 {
		return nil
	}
	const minTailBytes int64 = 64 * 1024
	estimated := int64(maxLines * 256)
	if estimated < minTailBytes {
		estimated = minTailBytes
	}
	offset := int64(0)
	if size > estimated {
		offset = size - estimated
	}
	if _, err := file.Seek(offset, 0); err != nil {
		return nil
	}
	buf := make([]byte, size-offset)
	n, err := file.Read(buf)
	if err != nil && n == 0 {
		return nil
	}
	content := string(buf[:n])
	rawLines := strings.Split(content, "\n")
	if offset > 0 && len(rawLines) > 0 {
		rawLines = rawLines[1:]
	}
	lines := make([]string, 0, maxLines)
	for _, raw := range rawLines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		lines = append(lines, normalizeHistoryLine(line))
		if len(lines) > maxLines {
			lines = lines[len(lines)-maxLines:]
		}
	}
	return lines
}

func historyPath() string {
	if hist := os.Getenv("HISTFILE"); hist != "" {
		return hist
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	candidates := []string{
		filepath.Join(home, ".zsh_history"),
		filepath.Join(home, ".bash_history"),
		filepath.Join(home, ".config", "fish", "fish_history"),
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func normalizeHistoryLine(line string) string {
	// zsh history format: ": 1680000000:0;command"
	if strings.HasPrefix(line, ": ") {
		if idx := strings.Index(line, ";"); idx != -1 {
			return strings.TrimSpace(line[idx+1:])
		}
	}
	// fish history uses "- cmd: <command>"
	if strings.HasPrefix(line, "- cmd: ") {
		return strings.TrimSpace(strings.TrimPrefix(line, "- cmd: "))
	}
	return line
}
