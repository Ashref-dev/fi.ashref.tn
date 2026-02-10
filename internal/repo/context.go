package repo

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"fi-cli/internal/util"
)

// Limits controls context size.
type Limits struct {
	ContextMaxBytes int
	MaxFileBytes    int
}

// FileSnippet holds a path and snippet text.
type FileSnippet struct {
	Path      string
	Snippet   string
	Truncated bool
}

// RepoContext summarizes repository metadata for prompting.
type RepoContext struct {
	RepoRoot            string
	TopLevel            []string
	KeyFiles            map[string]bool
	FrameworkIndicators map[string]bool
	Snippets            []FileSnippet
	Warnings            []string
	Bytes               int
}

// BuildContext gathers repo metadata and file snippets.
func BuildContext(repoRoot string, limits Limits) (RepoContext, error) {
	ctx := RepoContext{
		RepoRoot:            repoRoot,
		KeyFiles:            map[string]bool{},
		FrameworkIndicators: map[string]bool{},
	}

	entries, err := os.ReadDir(repoRoot)
	if err == nil {
		for _, entry := range entries {
			ctx.TopLevel = append(ctx.TopLevel, entry.Name())
		}
		sort.Strings(ctx.TopLevel)
	}

	keyFiles := []string{
		"package.json",
		"pnpm-lock.yaml",
		"yarn.lock",
		"package-lock.json",
		"go.mod",
		"Dockerfile",
		"docker-compose.yml",
		"Makefile",
		"README.md",
		".env.example",
		"tsconfig.json",
	}

	for _, name := range keyFiles {
		path := filepath.Join(repoRoot, name)
		if _, err := os.Stat(path); err == nil {
			ctx.KeyFiles[name] = true
		} else {
			ctx.KeyFiles[name] = false
		}
	}

	for _, name := range []string{"app", "pages", "src", "server", "api"} {
		path := filepath.Join(repoRoot, name)
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			ctx.FrameworkIndicators[name+"/"] = true
		} else {
			ctx.FrameworkIndicators[name+"/"] = false
		}
	}

	// next.config.* detection
	if matches, _ := filepath.Glob(filepath.Join(repoRoot, "next.config.*")); len(matches) > 0 {
		ctx.KeyFiles["next.config.*"] = true
		for _, match := range matches {
			_ = ctx.addSnippet(match, readFileLimited(match, limits.MaxFileBytes), limits)
		}
	} else {
		ctx.KeyFiles["next.config.*"] = false
	}

	if ctx.KeyFiles["package.json"] {
		path := filepath.Join(repoRoot, "package.json")
		if !IsDenylisted(path) {
			snippet := extractPackageJSON(path, limits.MaxFileBytes)
			_ = ctx.addSnippet(path, snippet, limits)
		}
	}

	if ctx.KeyFiles["README.md"] {
		path := filepath.Join(repoRoot, "README.md")
		if !IsDenylisted(path) {
			snippet := readFirstLines(path, 80, limits.MaxFileBytes)
			_ = ctx.addSnippet(path, snippet, limits)
		}
	}

	if ctx.KeyFiles["pnpm-lock.yaml"] {
		path := filepath.Join(repoRoot, "pnpm-lock.yaml")
		_ = ctx.addSnippet(path, readFirstLines(path, 40, limits.MaxFileBytes), limits)
	}
	if ctx.KeyFiles["yarn.lock"] {
		path := filepath.Join(repoRoot, "yarn.lock")
		_ = ctx.addSnippet(path, readFirstLines(path, 40, limits.MaxFileBytes), limits)
	}
	if ctx.KeyFiles["package-lock.json"] {
		path := filepath.Join(repoRoot, "package-lock.json")
		_ = ctx.addSnippet(path, readFirstLines(path, 40, limits.MaxFileBytes), limits)
	}
	if ctx.KeyFiles["go.mod"] {
		path := filepath.Join(repoRoot, "go.mod")
		_ = ctx.addSnippet(path, readFirstLines(path, 80, limits.MaxFileBytes), limits)
	}
	if ctx.KeyFiles["Dockerfile"] {
		path := filepath.Join(repoRoot, "Dockerfile")
		_ = ctx.addSnippet(path, readFirstLines(path, 80, limits.MaxFileBytes), limits)
	}
	if ctx.KeyFiles["docker-compose.yml"] {
		path := filepath.Join(repoRoot, "docker-compose.yml")
		_ = ctx.addSnippet(path, readFirstLines(path, 80, limits.MaxFileBytes), limits)
	}
	if ctx.KeyFiles["Makefile"] {
		path := filepath.Join(repoRoot, "Makefile")
		_ = ctx.addSnippet(path, readFirstLines(path, 80, limits.MaxFileBytes), limits)
	}
	if ctx.KeyFiles["tsconfig.json"] {
		path := filepath.Join(repoRoot, "tsconfig.json")
		_ = ctx.addSnippet(path, readFileLimited(path, limits.MaxFileBytes), limits)
	}

	if ctx.KeyFiles[".env.example"] {
		ctx.Warnings = append(ctx.Warnings, "Detected .env.example but contents are redacted by denylist policy.")
	}

	return ctx, nil
}

func (c *RepoContext) addSnippet(path string, raw string, limits Limits) error {
	if raw == "" {
		return nil
	}
	rel, _ := filepath.Rel(c.RepoRoot, path)
	redacted := util.RedactSecrets(raw)
	truncated := false
	if limits.ContextMaxBytes > 0 {
		remaining := limits.ContextMaxBytes - c.Bytes
		if remaining <= 0 {
			return nil
		}
		if len(redacted) > remaining {
			redacted = redacted[:remaining]
			truncated = true
		}
		c.Bytes += len(redacted)
	}
	c.Snippets = append(c.Snippets, FileSnippet{Path: rel, Snippet: redacted, Truncated: truncated})
	return nil
}

func readFileLimited(path string, maxBytes int) string {
	if IsDenylisted(path) {
		return ""
	}
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	limit := maxBytes
	if limit <= 0 {
		limit = 32 * 1024
	}
	buf := make([]byte, limit)
	n, _ := file.Read(buf)
	return string(buf[:n])
}

func readFirstLines(path string, maxLines int, maxBytes int) string {
	if IsDenylisted(path) {
		return ""
	}
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lines := []string{}
	bytes := 0
	for scanner.Scan() {
		line := scanner.Text()
		if maxLines > 0 && len(lines) >= maxLines {
			break
		}
		if maxBytes > 0 && bytes+len(line) > maxBytes {
			break
		}
		lines = append(lines, line)
		bytes += len(line)
	}
	return strings.Join(lines, "\n")
}

func extractPackageJSON(path string, maxBytes int) string {
	if IsDenylisted(path) {
		return ""
	}
	content := readFileLimited(path, maxBytes)
	if content == "" {
		return ""
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(content), &data); err != nil {
		return content
	}
	filtered := map[string]any{}
	for _, key := range []string{"name", "private", "packageManager", "scripts", "dependencies", "devDependencies", "peerDependencies"} {
		if val, ok := data[key]; ok {
			filtered[key] = val
		}
	}
	out, err := json.MarshalIndent(filtered, "", "  ")
	if err != nil {
		return content
	}
	return string(out)
}

// Summary renders a concise summary suitable for prompt context.
func (c RepoContext) Summary() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Repo root: %s\n", c.RepoRoot))
	if len(c.TopLevel) > 0 {
		b.WriteString("Top-level entries:\n")
		for _, entry := range c.TopLevel {
			b.WriteString("- ")
			b.WriteString(entry)
			b.WriteString("\n")
		}
	}
	if len(c.KeyFiles) > 0 {
		b.WriteString("Key files:\n")
		keys := make([]string, 0, len(c.KeyFiles))
		for k := range c.KeyFiles {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString(fmt.Sprintf("- %s: %t\n", k, c.KeyFiles[k]))
		}
	}
	if len(c.FrameworkIndicators) > 0 {
		b.WriteString("Framework indicators:\n")
		keys := make([]string, 0, len(c.FrameworkIndicators))
		for k := range c.FrameworkIndicators {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString(fmt.Sprintf("- %s: %t\n", k, c.FrameworkIndicators[k]))
		}
	}
	if len(c.Snippets) > 0 {
		b.WriteString("Snippets:\n")
		for _, snip := range c.Snippets {
			b.WriteString(fmt.Sprintf("--- %s", snip.Path))
			if snip.Truncated {
				b.WriteString(" (truncated)")
			}
			b.WriteString(" ---\n")
			b.WriteString(snip.Snippet)
			b.WriteString("\n")
		}
	}
	if len(c.Warnings) > 0 {
		b.WriteString("Warnings:\n")
		for _, warning := range c.Warnings {
			b.WriteString("- ")
			b.WriteString(warning)
			b.WriteString("\n")
		}
	}
	return b.String()
}
