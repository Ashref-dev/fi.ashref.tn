package repo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildContextComplexRepository(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "package.json"), `{
  "name": "complex-app",
  "scripts": {"dev":"next dev","build":"next build","test":"vitest"},
  "dependencies": {"next":"15.0.0","react":"19.0.0"},
  "devDependencies": {"typescript":"5.8.0"}
}`)
	mustWriteFile(t, filepath.Join(root, "README.md"), strings.Repeat("line\n", 120))
	mustWriteFile(t, filepath.Join(root, "next.config.mjs"), `export default {};`)
	mustWriteFile(t, filepath.Join(root, ".env.example"), "API_KEY=sample")
	mustWriteFile(t, filepath.Join(root, ".env"), "SECRET=should_not_be_read")
	mustWriteFile(t, filepath.Join(root, "tsconfig.json"), `{"compilerOptions":{"strict":true}}`)
	if err := os.MkdirAll(filepath.Join(root, "app", "api", "users"), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src", "services"), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	ctx, err := BuildContext(root, Limits{ContextMaxBytes: 512, MaxFileBytes: 1024})
	if err != nil {
		t.Fatalf("build context failed: %v", err)
	}
	if !ctx.KeyFiles["package.json"] {
		t.Fatalf("expected package.json detected")
	}
	if !ctx.KeyFiles["next.config.*"] {
		t.Fatalf("expected next.config.* detected")
	}
	if !ctx.FrameworkIndicators["app/"] || !ctx.FrameworkIndicators["src/"] {
		t.Fatalf("expected app/ and src/ indicators")
	}
	if ctx.Bytes > 512 {
		t.Fatalf("context bytes exceeded limit: %d", ctx.Bytes)
	}
	if len(ctx.Snippets) == 0 {
		t.Fatalf("expected snippets")
	}
	joined := ctx.Summary()
	if strings.Contains(joined, "SECRET=should_not_be_read") {
		t.Fatalf("summary should not include denylisted secrets")
	}
	if !strings.Contains(joined, "Detected .env.example") {
		t.Fatalf("expected .env.example warning")
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}
}
