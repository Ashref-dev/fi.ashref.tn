package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	DefaultModel             = "openrouter/pony-alpha"
	DefaultMaxSteps          = 8
	DefaultTimeout           = 60 * time.Second
	DefaultBaseURL           = "https://openrouter.ai/api/v1"
	DefaultResponseMode      = "quick"
	DefaultMaxContext        = 80 * 1024
	DefaultGrepLines         = 200
	DefaultGrepBytes         = 20 * 1024
	DefaultShellBytes        = 20 * 1024
	DefaultWebBytes          = 30 * 1024
	DefaultMaxFileSize       = 32 * 1024
	DefaultToolTimeout       = 10
	DefaultToolParallel      = 4
	DefaultReviewMaxFiles    = 20
	DefaultReviewMaxFindings = 25
)

// ToolLimits controls max output sizes for tools and context.
type ToolLimits struct {
	GrepMaxResults  int `mapstructure:"grep_max_results"`
	GrepMaxBytes    int `mapstructure:"grep_max_bytes"`
	ShellMaxBytes   int `mapstructure:"shell_max_bytes"`
	WebMaxBytes     int `mapstructure:"web_max_bytes"`
	GrepMaxCalls    int `mapstructure:"grep_max_calls"`
	ShellMaxCalls   int `mapstructure:"shell_max_calls"`
	WebMaxCalls     int `mapstructure:"web_max_calls"`
	ContextMaxBytes int `mapstructure:"context_max_bytes"`
	MaxFileBytes    int `mapstructure:"max_file_bytes"`
}

// Config holds runtime configuration values.
type Config struct {
	Model                      string
	FallbackModels             []string
	MaxSteps                   int
	Repo                       string
	APIKey                     string
	Timeout                    time.Duration
	ToolTimeoutSeconds         int
	ToolParallelism            int
	LLMRetryMaxAttempts        int
	LLMRetryInitialBackoff     time.Duration
	LLMRetryMaxBackoff         time.Duration
	LLMCircuitFailureThreshold int
	LLMCircuitWindow           time.Duration
	LLMCircuitOpenDuration     time.Duration
	UnsafeShell                bool
	ShellAllowlist             []string
	NoWeb                      bool
	NoPlan                     bool
	ShowHeader                 bool
	ShowTools                  bool
	NoTools                    bool
	ResponseMode               string
	Quiet                      bool
	JSON                       bool
	Timings                    bool
	Verbose                    bool
	LogFile                    string
	HistoryLines               int
	NoHistory                  bool
	OutputFormat               string
	PersistRuns                bool
	ReviewMaxFiles             int
	ReviewMaxFindings          int
	ReviewInstructionsFile     string
	ReviewPathRulesFile        string
	OpenRouterBaseURL          string
	HTTPReferer                string
	Title                      string
	ToolLimits                 ToolLimits
}

type rawConfig struct {
	Model                      string     `mapstructure:"model"`
	FallbackModels             []string   `mapstructure:"fallback_models"`
	MaxSteps                   int        `mapstructure:"max_steps"`
	Repo                       string     `mapstructure:"repo"`
	APIKey                     string     `mapstructure:"api_key"`
	Timeout                    string     `mapstructure:"timeout"`
	ToolTimeoutSeconds         int        `mapstructure:"tool_timeout_seconds"`
	ToolParallelism            int        `mapstructure:"tool_parallelism"`
	LLMRetryMaxAttempts        int        `mapstructure:"llm_retry_max_attempts"`
	LLMRetryInitialBackoff     string     `mapstructure:"llm_retry_initial_backoff"`
	LLMRetryMaxBackoff         string     `mapstructure:"llm_retry_max_backoff"`
	LLMCircuitFailureThreshold int        `mapstructure:"llm_circuit_failure_threshold"`
	LLMCircuitWindow           string     `mapstructure:"llm_circuit_window"`
	LLMCircuitOpenDuration     string     `mapstructure:"llm_circuit_open_duration"`
	UnsafeShell                bool       `mapstructure:"unsafe_shell"`
	UnsafeShellDefault         bool       `mapstructure:"unsafe_shell_default"`
	ShellAllowlist             []string   `mapstructure:"shell_allowlist"`
	NoWeb                      bool       `mapstructure:"no_web"`
	NoPlan                     bool       `mapstructure:"no_plan"`
	ShowHeader                 bool       `mapstructure:"show_header"`
	ShowTools                  bool       `mapstructure:"show_tools"`
	NoTools                    bool       `mapstructure:"no_tools"`
	ResponseMode               string     `mapstructure:"response_mode"`
	Quiet                      bool       `mapstructure:"quiet"`
	JSON                       bool       `mapstructure:"json"`
	Timings                    bool       `mapstructure:"timings"`
	Verbose                    bool       `mapstructure:"verbose"`
	LogFile                    string     `mapstructure:"log_file"`
	HistoryLines               int        `mapstructure:"history_lines"`
	NoHistory                  bool       `mapstructure:"no_history"`
	OutputFormat               string     `mapstructure:"output_format"`
	PersistRuns                bool       `mapstructure:"persist_runs"`
	ReviewMaxFiles             int        `mapstructure:"review_max_files"`
	ReviewMaxFindings          int        `mapstructure:"review_max_findings"`
	ReviewInstructionsFile     string     `mapstructure:"review_instructions_file"`
	ReviewPathRulesFile        string     `mapstructure:"review_path_rules_file"`
	OpenRouterBaseURL          string     `mapstructure:"openrouter_base_url"`
	HTTPReferer                string     `mapstructure:"http_referer"`
	Title                      string     `mapstructure:"title"`
	ToolLimits                 ToolLimits `mapstructure:"tool_limits"`
}

// Load resolves configuration from defaults, config files, env, and flags.
func Load(cmd *cobra.Command) (Config, error) {
	v := viper.New()
	v.SetEnvPrefix("FICLI")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	v.SetDefault("model", DefaultModel)
	v.SetDefault("fallback_models", []string{})
	v.SetDefault("max_steps", DefaultMaxSteps)
	v.SetDefault("timeout", DefaultTimeout.String())
	v.SetDefault("tool_timeout_seconds", DefaultToolTimeout)
	v.SetDefault("tool_parallelism", DefaultToolParallel)
	v.SetDefault("llm_retry_max_attempts", 2)
	v.SetDefault("llm_retry_initial_backoff", "300ms")
	v.SetDefault("llm_retry_max_backoff", "2s")
	v.SetDefault("llm_circuit_failure_threshold", 5)
	v.SetDefault("llm_circuit_window", "30s")
	v.SetDefault("llm_circuit_open_duration", "15s")
	v.SetDefault("repo", ".")
	v.SetDefault("api_key", "")
	v.SetDefault("unsafe_shell", false)
	v.SetDefault("unsafe_shell_default", false)
	v.SetDefault("shell_allowlist", []string{})
	v.SetDefault("no_web", false)
	v.SetDefault("no_plan", true)
	v.SetDefault("show_header", false)
	v.SetDefault("show_tools", true)
	v.SetDefault("no_tools", false)
	v.SetDefault("response_mode", DefaultResponseMode)
	v.SetDefault("quiet", false)
	v.SetDefault("json", false)
	v.SetDefault("timings", false)
	v.SetDefault("verbose", false)
	v.SetDefault("log_file", "")
	v.SetDefault("history_lines", 50)
	v.SetDefault("no_history", false)
	v.SetDefault("output_format", "text")
	v.SetDefault("persist_runs", false)
	v.SetDefault("review_max_files", DefaultReviewMaxFiles)
	v.SetDefault("review_max_findings", DefaultReviewMaxFindings)
	v.SetDefault("review_instructions_file", ".fi/review.md")
	v.SetDefault("review_path_rules_file", ".fi/review-paths.yaml")
	v.SetDefault("openrouter_base_url", DefaultBaseURL)
	v.SetDefault("tool_limits.grep_max_results", DefaultGrepLines)
	v.SetDefault("tool_limits.grep_max_bytes", DefaultGrepBytes)
	v.SetDefault("tool_limits.shell_max_bytes", DefaultShellBytes)
	v.SetDefault("tool_limits.web_max_bytes", DefaultWebBytes)
	v.SetDefault("tool_limits.grep_max_calls", 30)
	v.SetDefault("tool_limits.shell_max_calls", 30)
	v.SetDefault("tool_limits.web_max_calls", 30)
	v.SetDefault("tool_limits.context_max_bytes", DefaultMaxContext)
	v.SetDefault("tool_limits.max_file_bytes", DefaultMaxFileSize)

	if cmd != nil {
		bindFlagIfPresent(v, cmd, "model", "model")
		bindFlagIfPresent(v, cmd, "fallback_models", "fallback-model")
		bindFlagIfPresent(v, cmd, "max_steps", "max-steps")
		bindFlagIfPresent(v, cmd, "repo", "repo")
		bindFlagIfPresent(v, cmd, "timeout", "timeout")
		bindFlagIfPresent(v, cmd, "unsafe_shell", "unsafe-shell")
		bindFlagIfPresent(v, cmd, "no_web", "no-web")
		bindFlagIfPresent(v, cmd, "no_plan", "no-plan")
		bindFlagIfPresent(v, cmd, "show_header", "show-header")
		bindFlagIfPresent(v, cmd, "show_tools", "show-tools")
		bindFlagIfPresent(v, cmd, "no_tools", "no-tools")
		bindFlagIfPresent(v, cmd, "response_mode", "mode")
		bindFlagIfPresent(v, cmd, "quiet", "quiet")
		bindFlagIfPresent(v, cmd, "json", "json")
		bindFlagIfPresent(v, cmd, "timings", "timings")
		bindFlagIfPresent(v, cmd, "verbose", "verbose")
		bindFlagIfPresent(v, cmd, "log_file", "log-file")
		bindFlagIfPresent(v, cmd, "history_lines", "history-lines")
		bindFlagIfPresent(v, cmd, "no_history", "no-history")
		bindFlagIfPresent(v, cmd, "shell_allowlist", "shell-allow")
		bindFlagIfPresent(v, cmd, "review_max_files", "review-max-files")
		bindFlagIfPresent(v, cmd, "review_max_findings", "review-max-findings")
		bindFlagIfPresent(v, cmd, "review_instructions_file", "review-instructions-file")
		bindFlagIfPresent(v, cmd, "review_path_rules_file", "review-path-rules-file")
	}

	if seconds := os.Getenv("FICLI_TIMEOUT_SECONDS"); seconds != "" {
		v.Set("timeout", seconds+"s")
	}
	if model := os.Getenv("FICLI_MODEL"); model != "" {
		v.Set("model", model)
	}
	if fallbackModels := os.Getenv("FICLI_FALLBACK_MODELS"); fallbackModels != "" {
		v.Set("fallback_models", splitCSV(fallbackModels))
	}
	if baseURL := os.Getenv("FICLI_OPENROUTER_BASE_URL"); baseURL != "" {
		v.Set("openrouter_base_url", baseURL)
	}
	if allowlist := os.Getenv("FICLI_SHELL_ALLOWLIST"); allowlist != "" {
		v.Set("shell_allowlist", splitCSV(allowlist))
	}
	if value := os.Getenv("FICLI_REVIEW_INSTRUCTIONS_FILE"); value != "" {
		v.Set("review_instructions_file", value)
	}
	if value := os.Getenv("FICLI_REVIEW_PATH_RULES_FILE"); value != "" {
		v.Set("review_path_rules_file", value)
	}
	if openAIModel := os.Getenv("OPENAI_MODEL"); openAIModel != "" && os.Getenv("FICLI_MODEL") == "" {
		v.Set("model", openAIModel)
	}
	if openAIBaseURL := os.Getenv("OPENAI_BASE_URL"); openAIBaseURL != "" && os.Getenv("FICLI_OPENROUTER_BASE_URL") == "" {
		v.Set("openrouter_base_url", openAIBaseURL)
	}

	if err := loadConfigFile(v); err != nil {
		return Config{}, err
	}

	var raw rawConfig
	decoder, _ := mapstructure.NewDecoder(&mapstructure.DecoderConfig{TagName: "mapstructure", Result: &raw})
	if err := decoder.Decode(v.AllSettings()); err != nil {
		return Config{}, err
	}

	timeout := DefaultTimeout
	if raw.Timeout != "" {
		parsed, err := time.ParseDuration(raw.Timeout)
		if err != nil {
			return Config{}, fmt.Errorf("invalid timeout duration: %w", err)
		}
		timeout = parsed
	}
	retryInitialBackoff, err := parseDurationOrDefault(raw.LLMRetryInitialBackoff, 300*time.Millisecond)
	if err != nil {
		return Config{}, fmt.Errorf("invalid llm_retry_initial_backoff: %w", err)
	}
	retryMaxBackoff, err := parseDurationOrDefault(raw.LLMRetryMaxBackoff, 2*time.Second)
	if err != nil {
		return Config{}, fmt.Errorf("invalid llm_retry_max_backoff: %w", err)
	}
	circuitWindow, err := parseDurationOrDefault(raw.LLMCircuitWindow, 30*time.Second)
	if err != nil {
		return Config{}, fmt.Errorf("invalid llm_circuit_window: %w", err)
	}
	circuitOpenDuration, err := parseDurationOrDefault(raw.LLMCircuitOpenDuration, 15*time.Second)
	if err != nil {
		return Config{}, fmt.Errorf("invalid llm_circuit_open_duration: %w", err)
	}

	unsafeShell := raw.UnsafeShell
	if cmd != nil && cmd.Flags().Changed("unsafe-shell") {
		unsafeShell = v.GetBool("unsafe_shell")
	} else if v.IsSet("unsafe_shell_default") {
		unsafeShell = raw.UnsafeShellDefault
	}

	noPlan := raw.NoPlan
	if cmd != nil && cmd.Flags().Changed("plan") {
		if cmd.Flags().Lookup("plan").Value.String() == "true" {
			noPlan = false
		}
	}

	showTools := raw.ShowTools
	if cmd != nil && cmd.Flags().Changed("show-tools") {
		showTools = v.GetBool("show_tools")
	}
	if raw.NoTools {
		showTools = false
	}

	jsonOutput := raw.JSON
	if cmd != nil && cmd.Flags().Changed("json") {
		jsonOutput = v.GetBool("json")
	} else if strings.EqualFold(raw.OutputFormat, "json") {
		jsonOutput = true
	}

	cfg := Config{
		Model:                      raw.Model,
		FallbackModels:             normalizeAllowlist(raw.FallbackModels),
		MaxSteps:                   raw.MaxSteps,
		Repo:                       raw.Repo,
		APIKey:                     strings.TrimSpace(raw.APIKey),
		Timeout:                    timeout,
		ToolTimeoutSeconds:         raw.ToolTimeoutSeconds,
		ToolParallelism:            raw.ToolParallelism,
		LLMRetryMaxAttempts:        raw.LLMRetryMaxAttempts,
		LLMRetryInitialBackoff:     retryInitialBackoff,
		LLMRetryMaxBackoff:         retryMaxBackoff,
		LLMCircuitFailureThreshold: raw.LLMCircuitFailureThreshold,
		LLMCircuitWindow:           circuitWindow,
		LLMCircuitOpenDuration:     circuitOpenDuration,
		UnsafeShell:                unsafeShell,
		ShellAllowlist:             normalizeAllowlist(raw.ShellAllowlist),
		NoWeb:                      raw.NoWeb,
		NoPlan:                     noPlan,
		ShowHeader:                 raw.ShowHeader,
		ShowTools:                  showTools,
		NoTools:                    raw.NoTools,
		ResponseMode:               normalizeResponseMode(raw.ResponseMode),
		Quiet:                      raw.Quiet,
		JSON:                       jsonOutput,
		Timings:                    raw.Timings,
		Verbose:                    raw.Verbose,
		LogFile:                    raw.LogFile,
		HistoryLines:               raw.HistoryLines,
		NoHistory:                  raw.NoHistory,
		OutputFormat:               raw.OutputFormat,
		PersistRuns:                raw.PersistRuns,
		ReviewMaxFiles:             raw.ReviewMaxFiles,
		ReviewMaxFindings:          raw.ReviewMaxFindings,
		ReviewInstructionsFile:     raw.ReviewInstructionsFile,
		ReviewPathRulesFile:        raw.ReviewPathRulesFile,
		OpenRouterBaseURL:          raw.OpenRouterBaseURL,
		HTTPReferer:                raw.HTTPReferer,
		Title:                      raw.Title,
		ToolLimits:                 raw.ToolLimits,
	}

	if cfg.Model == "" {
		cfg.Model = DefaultModel
	}
	if cfg.MaxSteps <= 0 {
		cfg.MaxSteps = DefaultMaxSteps
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.ToolTimeoutSeconds <= 0 {
		cfg.ToolTimeoutSeconds = DefaultToolTimeout
	}
	if cfg.ToolParallelism <= 0 {
		cfg.ToolParallelism = DefaultToolParallel
	}
	if cfg.LLMRetryMaxAttempts < 0 {
		cfg.LLMRetryMaxAttempts = 0
	}
	if cfg.LLMRetryInitialBackoff <= 0 {
		cfg.LLMRetryInitialBackoff = 300 * time.Millisecond
	}
	if cfg.LLMRetryMaxBackoff <= 0 {
		cfg.LLMRetryMaxBackoff = 2 * time.Second
	}
	if cfg.LLMCircuitFailureThreshold <= 0 {
		cfg.LLMCircuitFailureThreshold = 5
	}
	if cfg.LLMCircuitWindow <= 0 {
		cfg.LLMCircuitWindow = 30 * time.Second
	}
	if cfg.LLMCircuitOpenDuration <= 0 {
		cfg.LLMCircuitOpenDuration = 15 * time.Second
	}
	if cfg.LLMRetryInitialBackoff > cfg.LLMRetryMaxBackoff {
		cfg.LLMRetryInitialBackoff = cfg.LLMRetryMaxBackoff
	}
	if cfg.OpenRouterBaseURL == "" {
		cfg.OpenRouterBaseURL = DefaultBaseURL
	}
	if cfg.HistoryLines < 0 {
		cfg.HistoryLines = 0
	}
	if cfg.ResponseMode == "" {
		cfg.ResponseMode = DefaultResponseMode
	}
	if cfg.ReviewMaxFiles <= 0 {
		cfg.ReviewMaxFiles = DefaultReviewMaxFiles
	}
	if cfg.ReviewMaxFindings <= 0 {
		cfg.ReviewMaxFindings = DefaultReviewMaxFindings
	}
	if strings.TrimSpace(cfg.ReviewInstructionsFile) == "" {
		cfg.ReviewInstructionsFile = ".fi/review.md"
	}
	if strings.TrimSpace(cfg.ReviewPathRulesFile) == "" {
		cfg.ReviewPathRulesFile = ".fi/review-paths.yaml"
	}

	if cfg.ToolLimits.ContextMaxBytes <= 0 {
		cfg.ToolLimits.ContextMaxBytes = DefaultMaxContext
	}
	if cfg.ToolLimits.GrepMaxResults <= 0 {
		cfg.ToolLimits.GrepMaxResults = DefaultGrepLines
	}
	if cfg.ToolLimits.GrepMaxBytes <= 0 {
		cfg.ToolLimits.GrepMaxBytes = DefaultGrepBytes
	}
	if cfg.ToolLimits.ShellMaxBytes <= 0 {
		cfg.ToolLimits.ShellMaxBytes = DefaultShellBytes
	}
	if cfg.ToolLimits.WebMaxBytes <= 0 {
		cfg.ToolLimits.WebMaxBytes = DefaultWebBytes
	}
	if cfg.ToolLimits.MaxFileBytes <= 0 {
		cfg.ToolLimits.MaxFileBytes = DefaultMaxFileSize
	}
	if cfg.ToolLimits.GrepMaxCalls <= 0 {
		cfg.ToolLimits.GrepMaxCalls = 30
	}
	if cfg.ToolLimits.ShellMaxCalls <= 0 {
		cfg.ToolLimits.ShellMaxCalls = 30
	}
	if cfg.ToolLimits.WebMaxCalls <= 0 {
		cfg.ToolLimits.WebMaxCalls = 30
	}

	return cfg, nil
}

func loadConfigFile(v *viper.Viper) error {
	for _, path := range ConfigCandidatePaths() {
		if _, err := os.Stat(path); err == nil {
			v.SetConfigFile(path)
			if err := v.ReadInConfig(); err != nil {
				return err
			}
			return nil
		}
	}
	return nil
}

func ConfigSearchBases() []string {
	var bases []string
	if configDir, err := os.UserConfigDir(); err == nil && configDir != "" {
		bases = append(bases,
			filepath.Join(configDir, "fi.ashref.tn"),
			filepath.Join(configDir, "fi-cli"),
		)
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		bases = append(bases,
			filepath.Join(xdg, "fi.ashref.tn"),
			filepath.Join(xdg, "fi-cli"),
		)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		bases = append(bases,
			filepath.Join(home, ".config", "fi.ashref.tn"),
			filepath.Join(home, ".config", "fi-cli"),
		)
	}
	return uniqStrings(bases)
}

func ConfigCandidatePaths() []string {
	var candidates []string
	for _, base := range ConfigSearchBases() {
		candidates = append(candidates,
			filepath.Join(base, "config.yaml"),
			filepath.Join(base, "config.yml"),
			filepath.Join(base, "config.json"),
		)
	}
	return uniqStrings(candidates)
}

func ExistingConfigPath() string {
	for _, path := range ConfigCandidatePaths() {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func PreferredConfigPath() string {
	for _, base := range ConfigSearchBases() {
		if strings.Contains(base, "fi.ashref.tn") {
			return filepath.Join(base, "config.yaml")
		}
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "config.yaml"
	}
	return filepath.Join(home, ".config", "fi.ashref.tn", "config.yaml")
}

func splitCSV(input string) []string {
	parts := strings.Split(input, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

func normalizeAllowlist(list []string) []string {
	seen := make(map[string]struct{}, len(list))
	out := make([]string, 0, len(list))
	for _, item := range list {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func normalizeResponseMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "quick", "":
		return "quick"
	case "operator":
		return "operator"
	case "explain":
		return "explain"
	default:
		return "quick"
	}
}

func uniqStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func parseDurationOrDefault(raw string, fallback time.Duration) (time.Duration, error) {
	if strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func bindFlagIfPresent(v *viper.Viper, cmd *cobra.Command, key string, flagName string) {
	if v == nil || cmd == nil {
		return
	}
	flag := cmd.Flags().Lookup(flagName)
	if flag == nil {
		return
	}
	_ = v.BindPFlag(key, flag)
}
