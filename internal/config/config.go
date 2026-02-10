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
	DefaultModel       = "openrouter/pony-alpha"
	DefaultMaxSteps    = 8
	DefaultTimeout     = 60 * time.Second
	DefaultBaseURL     = "https://openrouter.ai/api/v1"
	DefaultMaxContext  = 80 * 1024
	DefaultGrepLines   = 200
	DefaultGrepBytes   = 20 * 1024
	DefaultShellBytes  = 20 * 1024
	DefaultWebBytes    = 30 * 1024
	DefaultMaxFileSize = 32 * 1024
)

// ToolLimits controls max output sizes for tools and context.
type ToolLimits struct {
	GrepMaxResults  int `mapstructure:"grep_max_results"`
	GrepMaxBytes    int `mapstructure:"grep_max_bytes"`
	ShellMaxBytes   int `mapstructure:"shell_max_bytes"`
	WebMaxBytes     int `mapstructure:"web_max_bytes"`
	ContextMaxBytes int `mapstructure:"context_max_bytes"`
	MaxFileBytes    int `mapstructure:"max_file_bytes"`
}

// Config holds runtime configuration values.
type Config struct {
	Model             string
	MaxSteps          int
	Repo              string
	Timeout           time.Duration
	UnsafeShell       bool
	NoWeb             bool
	NoPlan            bool
	Quiet             bool
	JSON              bool
	Verbose           bool
	LogFile           string
	HistoryLines      int
	NoHistory         bool
	OutputFormat      string
	PersistRuns       bool
	OpenRouterBaseURL string
	HTTPReferer       string
	Title             string
	ToolLimits        ToolLimits
}

type rawConfig struct {
	Model              string     `mapstructure:"model"`
	MaxSteps           int        `mapstructure:"max_steps"`
	Repo               string     `mapstructure:"repo"`
	Timeout            string     `mapstructure:"timeout"`
	UnsafeShell        bool       `mapstructure:"unsafe_shell"`
	UnsafeShellDefault bool       `mapstructure:"unsafe_shell_default"`
	NoWeb              bool       `mapstructure:"no_web"`
	NoPlan             bool       `mapstructure:"no_plan"`
	Quiet              bool       `mapstructure:"quiet"`
	JSON               bool       `mapstructure:"json"`
	Verbose            bool       `mapstructure:"verbose"`
	LogFile            string     `mapstructure:"log_file"`
	HistoryLines       int        `mapstructure:"history_lines"`
	NoHistory          bool       `mapstructure:"no_history"`
	OutputFormat       string     `mapstructure:"output_format"`
	PersistRuns        bool       `mapstructure:"persist_runs"`
	OpenRouterBaseURL  string     `mapstructure:"openrouter_base_url"`
	HTTPReferer        string     `mapstructure:"http_referer"`
	Title              string     `mapstructure:"title"`
	ToolLimits         ToolLimits `mapstructure:"tool_limits"`
}

// Load resolves configuration from defaults, config files, env, and flags.
func Load(cmd *cobra.Command) (Config, error) {
	v := viper.New()
	v.SetEnvPrefix("FI")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	v.SetDefault("model", DefaultModel)
	v.SetDefault("max_steps", DefaultMaxSteps)
	v.SetDefault("timeout", DefaultTimeout.String())
	v.SetDefault("repo", ".")
	v.SetDefault("unsafe_shell", false)
	v.SetDefault("unsafe_shell_default", false)
	v.SetDefault("no_web", false)
	v.SetDefault("no_plan", false)
	v.SetDefault("quiet", false)
	v.SetDefault("json", false)
	v.SetDefault("verbose", false)
	v.SetDefault("log_file", "")
	v.SetDefault("history_lines", 50)
	v.SetDefault("no_history", false)
	v.SetDefault("output_format", "text")
	v.SetDefault("persist_runs", false)
	v.SetDefault("openrouter_base_url", DefaultBaseURL)
	v.SetDefault("tool_limits.grep_max_results", DefaultGrepLines)
	v.SetDefault("tool_limits.grep_max_bytes", DefaultGrepBytes)
	v.SetDefault("tool_limits.shell_max_bytes", DefaultShellBytes)
	v.SetDefault("tool_limits.web_max_bytes", DefaultWebBytes)
	v.SetDefault("tool_limits.context_max_bytes", DefaultMaxContext)
	v.SetDefault("tool_limits.max_file_bytes", DefaultMaxFileSize)

	if cmd != nil {
		_ = v.BindPFlag("model", cmd.Flags().Lookup("model"))
		_ = v.BindPFlag("max_steps", cmd.Flags().Lookup("max-steps"))
		_ = v.BindPFlag("repo", cmd.Flags().Lookup("repo"))
		_ = v.BindPFlag("timeout", cmd.Flags().Lookup("timeout"))
		_ = v.BindPFlag("unsafe_shell", cmd.Flags().Lookup("unsafe-shell"))
		_ = v.BindPFlag("no_web", cmd.Flags().Lookup("no-web"))
		_ = v.BindPFlag("no_plan", cmd.Flags().Lookup("no-plan"))
		_ = v.BindPFlag("quiet", cmd.Flags().Lookup("quiet"))
		_ = v.BindPFlag("json", cmd.Flags().Lookup("json"))
		_ = v.BindPFlag("verbose", cmd.Flags().Lookup("verbose"))
		_ = v.BindPFlag("log_file", cmd.Flags().Lookup("log-file"))
		_ = v.BindPFlag("history_lines", cmd.Flags().Lookup("history-lines"))
		_ = v.BindPFlag("no_history", cmd.Flags().Lookup("no-history"))
	}

	if seconds := os.Getenv("FI_TIMEOUT_SECONDS"); seconds != "" {
		v.Set("timeout", seconds+"s")
	}
	if fiModel := os.Getenv("FI_MODEL"); fiModel != "" {
		v.Set("model", fiModel)
	}
	if fiBaseURL := os.Getenv("FI_BASE_URL"); fiBaseURL != "" {
		v.Set("openrouter_base_url", fiBaseURL)
	}
	if seconds := os.Getenv("AGCLI_TIMEOUT_SECONDS"); seconds != "" && os.Getenv("FI_TIMEOUT_SECONDS") == "" {
		v.Set("timeout", seconds+"s")
	}
	if agModel := os.Getenv("AGCLI_MODEL"); agModel != "" && os.Getenv("FI_MODEL") == "" {
		v.Set("model", agModel)
	}
	if agBaseURL := os.Getenv("AGCLI_OPENROUTER_BASE_URL"); agBaseURL != "" && os.Getenv("FI_BASE_URL") == "" {
		v.Set("openrouter_base_url", agBaseURL)
	}
	if openAIModel := os.Getenv("OPENAI_MODEL"); openAIModel != "" && os.Getenv("FI_MODEL") == "" && os.Getenv("AGCLI_MODEL") == "" {
		v.Set("model", openAIModel)
	}
	if openAIBaseURL := os.Getenv("OPENAI_BASE_URL"); openAIBaseURL != "" && os.Getenv("FI_BASE_URL") == "" && os.Getenv("AGCLI_OPENROUTER_BASE_URL") == "" {
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

	unsafeShell := raw.UnsafeShell
	if cmd != nil && cmd.Flags().Changed("unsafe-shell") {
		unsafeShell = v.GetBool("unsafe_shell")
	} else if v.IsSet("unsafe_shell_default") {
		unsafeShell = raw.UnsafeShellDefault
	}

	jsonOutput := raw.JSON
	if cmd != nil && cmd.Flags().Changed("json") {
		jsonOutput = v.GetBool("json")
	} else if strings.EqualFold(raw.OutputFormat, "json") {
		jsonOutput = true
	}

	cfg := Config{
		Model:             raw.Model,
		MaxSteps:          raw.MaxSteps,
		Repo:              raw.Repo,
		Timeout:           timeout,
		UnsafeShell:       unsafeShell,
		NoWeb:             raw.NoWeb,
		NoPlan:            raw.NoPlan,
		Quiet:             raw.Quiet,
		JSON:              jsonOutput,
		Verbose:           raw.Verbose,
		LogFile:           raw.LogFile,
		HistoryLines:      raw.HistoryLines,
		NoHistory:         raw.NoHistory,
		OutputFormat:      raw.OutputFormat,
		PersistRuns:       raw.PersistRuns,
		OpenRouterBaseURL: raw.OpenRouterBaseURL,
		HTTPReferer:       raw.HTTPReferer,
		Title:             raw.Title,
		ToolLimits:        raw.ToolLimits,
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
	if cfg.OpenRouterBaseURL == "" {
		cfg.OpenRouterBaseURL = DefaultBaseURL
	}
	if cfg.HistoryLines < 0 {
		cfg.HistoryLines = 0
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

	return cfg, nil
}

func loadConfigFile(v *viper.Viper) error {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil
	}
	bases := []string{
		filepath.Join(configDir, "fi-cli"),
		filepath.Join(configDir, "fi-cli"),
	}
	var candidates []string
	for _, base := range bases {
		candidates = append(candidates,
			filepath.Join(base, "config.yaml"),
			filepath.Join(base, "config.yml"),
			filepath.Join(base, "config.json"),
		)
	}

	for _, path := range candidates {
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
