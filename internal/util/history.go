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
