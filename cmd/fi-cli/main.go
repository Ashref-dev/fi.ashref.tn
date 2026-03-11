package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

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
		Use:           "fi-cli [question]",
		Short:         "fi-cli - terminal-native agent orchestrator",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			stageTimings := map[string]int64{}

			questionStart := time.Now()
			question, err := resolveQuestion(args, os.Stdin, os.Stdout)
			stageTimings[agent.StageQuestionResolutionInput] = durationMillis(time.Since(questionStart))
			if err != nil {
				if errors.Is(err, errNoQuestionInput) {
					return cmd.Help()
				}
				return err
			}

			configStart := time.Now()
			cfg, err := config.Load(cmd)
			stageTimings[agent.StageConfigLoad] = durationMillis(time.Since(configStart))
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
				fmt.Fprintf(os.Stderr, "fi-cli onboarding required.\n1) Run: fi-cli init\n2) Add api_key in: %s\n3) Run: fi-cli \"your question\"\n", onboardingPath)
				os.Exit(2)
			}

			logger := buildLogger(cfg.Verbose)
			defer func() { _ = logger.Sync() }()

			repoRootStart := time.Now()
			repoRoot, err := repo.FindRoot(cfg.Repo)
			stageTimings[agent.StageRepoRootResolution] = durationMillis(time.Since(repoRootStart))
			if err != nil {
				logger.Warn("failed to find repo root", zap.Error(err))
				repoRoot = cfg.Repo
			}
			repoRoot, _ = filepath.Abs(repoRoot)

			repoCtxStart := time.Now()
			repoCtx, err := repo.BuildContext(repoRoot, repo.Limits{ContextMaxBytes: cfg.ToolLimits.ContextMaxBytes, MaxFileBytes: cfg.ToolLimits.MaxFileBytes})
			stageTimings[agent.StageRepoContextBuild] = durationMillis(time.Since(repoCtxStart))
			if err != nil {
				logger.Warn("failed to build repo context", zap.Error(err))
			}

			grepTool := tools.NewGrepTool()
			treeTool := tools.NewListTreeTool()
			toolList := []tools.Tool{grepTool, treeTool}
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
			client = llm.NewResilientClient(client, llm.ResilienceConfig{
				FallbackModels:          cfg.FallbackModels,
				RetryMaxAttempts:        cfg.LLMRetryMaxAttempts,
				RetryInitialBackoff:     cfg.LLMRetryInitialBackoff,
				RetryMaxBackoff:         cfg.LLMRetryMaxBackoff,
				CircuitFailureThreshold: cfg.LLMCircuitFailureThreshold,
				CircuitWindow:           cfg.LLMCircuitWindow,
				CircuitOpenDuration:     cfg.LLMCircuitOpenDuration,
			})

			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()
			ctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
			defer cancel()

			ag := agent.NewAgent(client, registry, nil, logger, cfg)

			if cfg.JSON {
				result, err := ag.Run(ctx, question, repoRoot, repoCtx)
				mergeStageTimings(&result, stageTimings)
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
			renderer := render.NewStdoutRenderer(
				writer,
				cfg.Verbose,
				cfg.Quiet,
				cfg.NoPlan,
				cfg.ShowHeader,
				cfg.ShowTools,
				render.StdoutRendererOptions{
					Interactive:   isInteractiveTerminal(os.Stdout),
					SpinnerWriter: os.Stdout,
				},
			)
			ag = agent.NewAgent(client, registry, renderer, logger, cfg)
			runResult, runErr := ag.Run(ctx, question, repoRoot, repoCtx)
			_ = renderer.Close()
			mergeStageTimings(&runResult, stageTimings)
			if cfg.PersistRuns {
				persistRun(logger, runResult)
			}
			if cfg.Timings {
				printTimingSummary(writer, runResult.StageTimingsMs)
			}
			if logFile != nil {
				_ = logFile.Close()
			}
			if runErr != nil {
				return runErr
			}
			return nil
		},
	}

	cmd.Flags().String("model", config.DefaultModel, "Model name")
	cmd.Flags().StringSlice("fallback-model", nil, "Fallback model (repeatable)")
	cmd.Flags().String("mode", config.DefaultResponseMode, "Response mode: quick|operator|explain")
	cmd.Flags().Int("max-steps", config.DefaultMaxSteps, "Maximum tool steps")
	cmd.Flags().String("repo", ".", "Repository path")
	cmd.Flags().String("timeout", config.DefaultTimeout.String(), "Timeout (e.g. 60s)")
	cmd.Flags().Bool("unsafe-shell", false, "Allow unsafe shell commands")
	cmd.Flags().StringSlice("shell-allow", nil, "Allow shell command prefix (repeatable)")
	cmd.Flags().Bool("plan", false, "Generate and show a short plan")
	cmd.Flags().Bool("no-web", false, "Disable web search")
	cmd.Flags().Bool("no-plan", true, "Disable plan output and generation")
	cmd.Flags().Bool("timings", false, "Print stage timing diagnostics")
	cmd.Flags().Bool("show-header", false, "Show header lines")
	cmd.Flags().Bool("show-tools", true, "Show tool call summaries")
	cmd.Flags().Bool("no-tools", false, "Hide tool call summaries")
	cmd.Flags().Bool("quiet", false, "Only print final answer")
	cmd.Flags().Bool("json", false, "Output JSON only")
	cmd.Flags().Bool("verbose", false, "Enable verbose logging")
	cmd.Flags().String("log-file", "", "Write plain-text output to a file")
	cmd.Flags().Int("history-lines", 50, "Number of shell history lines to include")
	cmd.Flags().Bool("no-history", false, "Disable shell history context")
	_ = cmd.Flags().MarkHidden("no-plan")

	cmd.AddCommand(newInitCmd())
	cmd.AddCommand(newAboutCmd())
	cmd.AddCommand(newPolicyCmd())

	return cmd
}

func newInitCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize fi-cli config file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			target := config.ExistingConfigPath()
			if target == "" {
				target = config.PreferredConfigPath()
			} else if !force {
				fmt.Fprintf(os.Stdout, "Config already exists: %s\n", target)
				fmt.Fprintln(os.Stdout, "Use --force to overwrite. Next: set api_key and run `fi-cli \"your question\"`.")
				return nil
			}

			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}

			content := strings.TrimSpace(`
# fi-cli configuration
api_key: ""
model: openrouter/pony-alpha
# fallback_models:
#   - openrouter/anthropic/claude-3.5-sonnet
#   - openrouter/openai/gpt-4o-mini
openrouter_base_url: "https://openrouter.ai/api/v1"
response_mode: quick
show_header: false
show_tools: true
no_plan: true
llm_retry_max_attempts: 2
llm_retry_initial_backoff: 300ms
llm_retry_max_backoff: 2s
llm_circuit_failure_threshold: 5
llm_circuit_window: 30s
llm_circuit_open_duration: 15s
tool_parallelism: 4
tool_timeout_seconds: 10
tool_limits:
  grep_max_calls: 30
  shell_max_calls: 30
  web_max_calls: 30
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
			fmt.Fprintln(os.Stdout, "2) Optional shorthand alias: alias fic='fi-cli'")
			fmt.Fprintln(os.Stdout, "3) Run: fi-cli \"what's the tech stack here?\"")
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

func newAboutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "about",
		Short: "Show product, safety, and configuration details",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(nil)
			if err != nil {
				return err
			}
			mode := policy.ResolveShellMode(cfg.UnsafeShell, cfg.ShellAllowlist)
			fmt.Fprintln(os.Stdout, "fi-cli")
			fmt.Fprintln(os.Stdout, "- Default behavior: read-only repository analysis (grep/list_tree/context)")
			fmt.Fprintf(os.Stdout, "- Active shell mode: %s\n", mode)
			fmt.Fprintf(os.Stdout, "- Tool call caps: grep=%d shell=%d web=%d\n", cfg.ToolLimits.GrepMaxCalls, cfg.ToolLimits.ShellMaxCalls, cfg.ToolLimits.WebMaxCalls)
			fmt.Fprintf(os.Stdout, "- Response mode: %s\n", cfg.ResponseMode)
			fmt.Fprintln(os.Stdout, "- Config search order:")
			for _, candidate := range config.ConfigCandidatePaths() {
				fmt.Fprintf(os.Stdout, "  - %s\n", candidate)
			}
			return nil
		},
	}
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

var errNoQuestionInput = errors.New("no question provided")

func resolveQuestion(args []string, stdin *os.File, stdout io.Writer) (string, error) {
	if len(args) > 0 {
		return strings.Join(args, " "), nil
	}
	info, err := stdin.Stat()
	if err == nil && (info.Mode()&os.ModeCharDevice) == 0 {
		data, readErr := io.ReadAll(stdin)
		if readErr != nil {
			return "", readErr
		}
		question := strings.TrimSpace(string(data))
		if question == "" {
			return "", errNoQuestionInput
		}
		return question, nil
	}

	fmt.Fprint(stdout, "question> ")
	reader := bufio.NewReader(stdin)
	line, readErr := reader.ReadString('\n')
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return "", readErr
	}
	question := strings.TrimSpace(line)
	if question == "" {
		return "", errNoQuestionInput
	}
	return question, nil
}

func mergeStageTimings(result *agent.RunResult, preRunTimings map[string]int64) {
	if result.StageTimingsMs == nil {
		result.StageTimingsMs = agent.DefaultStageTimings()
	}
	for key, value := range preRunTimings {
		if value < 0 {
			value = 0
		}
		result.StageTimingsMs[key] = value
	}
}

func isInteractiveTerminal(file *os.File) bool {
	if file == nil {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func durationMillis(d time.Duration) int64 {
	if d < 0 {
		return 0
	}
	return d.Milliseconds()
}

func printTimingSummary(w io.Writer, stageTimings map[string]int64) {
	if w == nil {
		return
	}

	merged := agent.DefaultStageTimings()
	for key, value := range stageTimings {
		if value < 0 {
			value = 0
		}
		merged[key] = value
	}

	ordered := []string{
		agent.StageQuestionResolutionInput,
		agent.StageConfigLoad,
		agent.StageRepoRootResolution,
		agent.StageRepoContextBuild,
		agent.StageHistoryLoad,
		agent.StagePlanning,
		agent.StageFirstModelResponseWait,
		agent.StageToolExecutionTotal,
		agent.StageFirstAnswerTokenLatency,
		agent.StageTotalRunDuration,
	}

	fmt.Fprintln(w, "\ntimings:")
	for _, key := range ordered {
		fmt.Fprintf(w, "  %s: %dms\n", key, merged[key])
	}

	var toolStages []string
	for key := range merged {
		if strings.HasPrefix(key, agent.StageToolExecutionPrefix) {
			toolStages = append(toolStages, key)
		}
	}
	sort.Strings(toolStages)
	for _, key := range toolStages {
		fmt.Fprintf(w, "  %s: %dms\n", key, merged[key])
	}

	slowestKey := ""
	var slowestMs int64
	for key, value := range merged {
		if key == agent.StageTotalRunDuration || value < 0 {
			continue
		}
		if slowestKey == "" || value > slowestMs {
			slowestKey = key
			slowestMs = value
		}
	}
	if slowestKey == "" {
		slowestKey = "n/a"
	}
	fmt.Fprintf(w, "  slowest_stage: %s (%dms)\n", slowestKey, slowestMs)
}
