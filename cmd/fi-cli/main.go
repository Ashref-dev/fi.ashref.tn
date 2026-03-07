package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"fi-cli/internal/agent"
	"fi-cli/internal/config"
	"fi-cli/internal/llm"
	"fi-cli/internal/policy"
	"fi-cli/internal/render"
	"fi-cli/internal/repo"
	"fi-cli/internal/tools"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func main() {
	root := newRootCmd()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "vcli [question]",
		Short:         "V-CLI - terminal-native agent orchestrator",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			question := strings.Join(args, " ")
			cfg, err := config.Load(cmd)
			if err != nil {
				return err
			}
			if cfg.Quiet {
				cfg.NoPlan = true
				cfg.ShowHeader = false
				cfg.ShowTools = false
			}
			if cfg.Verbose {
				cfg.ShowTools = true
			}

			apiKey := os.Getenv("FICLI_API_KEY")
			if apiKey == "" {
				apiKey = os.Getenv("OPENROUTER_API_KEY")
			}
			if apiKey == "" {
				apiKey = os.Getenv("OPENAI_API_KEY")
			}
			if apiKey == "" {
				apiKey = cfg.APIKey
			}
			mockMode := os.Getenv("FICLI_MOCK_LLM") == "1"
			if apiKey == "" && !mockMode {
				onboardingPath := config.PreferredConfigPath()
				fmt.Fprintf(os.Stderr, "V-CLI onboarding required.\n1) Run: vcli init\n2) Add api_key in: %s\n3) Run: vcli \"your question\"\n", onboardingPath)
				os.Exit(2)
			}

			logger := buildLogger(cfg.Verbose)
			defer func() { _ = logger.Sync() }()

			repoRoot, err := repo.FindRoot(cfg.Repo)
			if err != nil {
				logger.Warn("failed to find repo root", zap.Error(err))
				repoRoot = cfg.Repo
			}
			repoRoot, _ = filepath.Abs(repoRoot)

			repoCtx, err := repo.BuildContext(repoRoot, repo.Limits{ContextMaxBytes: cfg.ToolLimits.ContextMaxBytes, MaxFileBytes: cfg.ToolLimits.MaxFileBytes})
			if err != nil {
				logger.Warn("failed to build repo context", zap.Error(err))
			}

			grepTool := tools.NewGrepTool()
			toolList := []tools.Tool{grepTool}
			if cfg.UnsafeShell || len(cfg.ShellAllowlist) > 0 {
				toolList = append(toolList, tools.NewShellTool(cfg.ShellAllowlist))
			}

			exaKey := os.Getenv("EXA_API_KEY")
			if exaKey != "" && !cfg.NoWeb {
				toolList = append(toolList, tools.NewExaTool(exaKey))
			} else {
				cfg.NoWeb = true
			}

			registry := tools.NewRegistry(toolList...)

			var client llm.Client
			if mockMode {
				client = llm.NewMockClient()
			} else {
				client = llm.NewOpenRouterClient(apiKey, cfg.OpenRouterBaseURL, cfg.HTTPReferer, cfg.Title)
			}

			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()
			ctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
			defer cancel()

			ag := agent.NewAgent(client, registry, nil, logger, cfg)

			if cfg.JSON {
				result, err := ag.Run(ctx, question, repoRoot, repoCtx)
				if cfg.PersistRuns {
					persistRun(logger, result)
					// ensure persistence failure doesn't block output
				}
				payload, _ := json.MarshalIndent(result, "", "  ")
				fmt.Fprintln(os.Stdout, string(payload))
				if err != nil {
					return err
				}
				return nil
			}

			writer := io.Writer(os.Stdout)
			var logFile *os.File
			if cfg.LogFile != "" {
				logPath := cfg.LogFile
				if !filepath.IsAbs(logPath) {
					logPath = filepath.Join(repoRoot, logPath)
				}
				file, err := os.Create(logPath)
				if err != nil {
					return err
				}
				logFile = file
				writer = io.MultiWriter(os.Stdout, logFile)
			}
			renderer := render.NewStdoutRenderer(writer, cfg.Verbose, cfg.Quiet, cfg.NoPlan, cfg.ShowHeader, cfg.ShowTools)
			ag = agent.NewAgent(client, registry, renderer, logger, cfg)
			runResult, runErr := ag.Run(ctx, question, repoRoot, repoCtx)
			_ = renderer.Close()
			if logFile != nil {
				_ = logFile.Close()
			}
			if cfg.PersistRuns {
				persistRun(logger, runResult)
			}
			if runErr != nil {
				return runErr
			}
			return nil
		},
	}

	cmd.Flags().String("model", config.DefaultModel, "Model name")
	cmd.Flags().String("mode", config.DefaultResponseMode, "Response mode: quick|operator|explain")
	cmd.Flags().Int("max-steps", config.DefaultMaxSteps, "Maximum tool steps")
	cmd.Flags().String("repo", ".", "Repository path")
	cmd.Flags().String("timeout", config.DefaultTimeout.String(), "Timeout (e.g. 60s)")
	cmd.Flags().Bool("unsafe-shell", false, "Allow unsafe shell commands")
	cmd.Flags().StringSlice("shell-allow", nil, "Allow shell command prefix (repeatable)")
	cmd.Flags().Bool("plan", false, "Generate and show a short plan")
	cmd.Flags().Bool("no-web", false, "Disable web search")
	cmd.Flags().Bool("no-plan", true, "Disable plan output and generation")
	cmd.Flags().Bool("show-header", false, "Show header lines")
	cmd.Flags().Bool("show-tools", true, "Show tool call summaries")
	cmd.Flags().Bool("no-tools", false, "Hide tool call summaries")
	cmd.Flags().Bool("quiet", false, "Only print final answer")
	cmd.Flags().Bool("json", false, "Output JSON only")
	cmd.Flags().Bool("verbose", false, "Enable verbose logging")
	cmd.Flags().String("log-file", "", "Write plain-text output to a file")
	cmd.Flags().Int("history-lines", 50, "Number of shell history lines to include")
	cmd.Flags().Bool("no-history", false, "Disable shell history context")

	cmd.AddCommand(newInitCmd())
	cmd.AddCommand(newPolicyCmd())

	return cmd
}

func newInitCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize V-CLI config file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			target := config.ExistingConfigPath()
			if target == "" {
				target = config.PreferredConfigPath()
			} else if !force {
				fmt.Fprintf(os.Stdout, "Config already exists: %s\n", target)
				fmt.Fprintln(os.Stdout, "Use --force to overwrite. Next: set api_key and run `vcli \"your question\"`.")
				return nil
			}

			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}

			content := strings.TrimSpace(`
# V-CLI configuration
api_key: ""
model: openrouter/pony-alpha
openrouter_base_url: "https://openrouter.ai/api/v1"
response_mode: quick
show_header: false
show_tools: true
no_plan: true
# shell_allowlist:
#   - git status
#   - git log
`) + "\n"

			if err := os.WriteFile(target, []byte(content), 0o600); err != nil {
				return err
			}

			fmt.Fprintf(os.Stdout, "Initialized config: %s\n", target)
			fmt.Fprintln(os.Stdout, "Next steps:")
			fmt.Fprintln(os.Stdout, "1) Set `api_key` in the config file")
			fmt.Fprintln(os.Stdout, "2) Optional shell alias: alias v='vcli'")
			fmt.Fprintln(os.Stdout, "3) Run: vcli \"what's the tech stack here?\"")
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing config")
	return cmd
}

func newPolicyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "policy",
		Short: "Inspect shell safety policy",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "check",
		Short: "Show current safety mode and allowlist",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(nil)
			if err != nil {
				return err
			}
			mode := policy.ResolveShellMode(cfg.UnsafeShell, cfg.ShellAllowlist)
			fmt.Fprintf(os.Stdout, "mode: %s\n", mode)
			fmt.Fprintf(os.Stdout, "shell_enabled: %t\n", mode != policy.ShellModeReadOnly)
			fmt.Fprintf(os.Stdout, "allowlist_entries: %d\n", len(cfg.ShellAllowlist))
			for _, entry := range cfg.ShellAllowlist {
				fmt.Fprintf(os.Stdout, "- %s\n", entry)
			}
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "test <command>",
		Short: "Test whether a shell command is allowed by current policy",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(nil)
			if err != nil {
				return err
			}
			command := strings.Join(args, " ")
			decision := policy.EvaluateShellCommand(command, cfg.UnsafeShell, cfg.ShellAllowlist)
			fmt.Fprintf(os.Stdout, "mode: %s\n", decision.Mode)
			fmt.Fprintf(os.Stdout, "allowed: %t\n", decision.Allowed)
			fmt.Fprintf(os.Stdout, "reason: %s\n", decision.Reason)
			return nil
		},
	})
	return cmd
}

func buildLogger(verbose bool) *zap.Logger {
	if verbose {
		logger, _ := zap.NewDevelopment()
		return logger
	}
	logger, _ := zap.NewProduction()
	return logger
}

func persistRun(logger *zap.Logger, result agent.RunResult) {
	home, err := os.UserHomeDir()
	if err != nil {
		logger.Warn("failed to get home dir", zap.Error(err))
		return
	}
	path := filepath.Join(home, ".local", "share", "fi.ashref.tn", "runs")
	if err := os.MkdirAll(path, 0o755); err != nil {
		logger.Warn("failed to create run directory", zap.Error(err))
		return
	}
	file := filepath.Join(path, result.RunID+".json")
	payload, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		logger.Warn("failed to marshal run log", zap.Error(err))
		return
	}
	if err := os.WriteFile(file, payload, 0o600); err != nil {
		logger.Warn("failed to write run log", zap.Error(err))
	}
}
