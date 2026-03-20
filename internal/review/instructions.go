package review

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

type pathRule struct {
	Glob         string `yaml:"glob"`
	Instructions string `yaml:"instructions"`
}

type pathRulesFile struct {
	Rules []pathRule `yaml:"rules"`
}

func loadReviewInstructions(repoRoot string, configuredPath string, defaultPath string) (string, error) {
	resolved, err := resolveReviewFilePath(repoRoot, configuredPath)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(resolved)
	if errors.Is(err, os.ErrNotExist) && sameReviewDefault(configuredPath, defaultPath) {
		return "", nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func loadPathRules(repoRoot string, configuredPath string, defaultPath string) ([]pathRule, error) {
	resolved, err := resolveReviewFilePath(repoRoot, configuredPath)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(resolved)
	if errors.Is(err, os.ErrNotExist) && sameReviewDefault(configuredPath, defaultPath) {
		return nil, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	var parsed pathRulesFile
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	rules := make([]pathRule, 0, len(parsed.Rules))
	for _, rule := range parsed.Rules {
		if strings.TrimSpace(rule.Glob) == "" || strings.TrimSpace(rule.Instructions) == "" {
			continue
		}
		rules = append(rules, pathRule{
			Glob:         filepath.ToSlash(strings.TrimSpace(rule.Glob)),
			Instructions: strings.TrimSpace(rule.Instructions),
		})
	}
	return rules, nil
}

func resolvePathInstructions(rules []pathRule, relPath string) string {
	relPath = filepath.ToSlash(strings.TrimSpace(relPath))
	var matches []string
	for _, rule := range rules {
		ok, err := path.Match(rule.Glob, relPath)
		if err != nil || !ok {
			continue
		}
		matches = append(matches, rule.Instructions)
	}
	return strings.Join(matches, "\n\n")
}

func resolveReviewFilePath(repoRoot string, configured string) (string, error) {
	value := strings.TrimSpace(configured)
	if value == "" {
		return "", errors.New("review file path is required")
	}
	if filepath.IsAbs(value) {
		return value, nil
	}
	return filepath.Join(repoRoot, filepath.Clean(value)), nil
}

func sameReviewDefault(configuredPath string, defaultPath string) bool {
	return filepath.Clean(strings.TrimSpace(configuredPath)) == filepath.Clean(strings.TrimSpace(defaultPath))
}
