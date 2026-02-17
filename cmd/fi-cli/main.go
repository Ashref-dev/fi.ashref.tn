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
		Use:           "fi [question]",
		Short:         "fi - terminal-native agent orchestrator",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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
				fmt.Fprintln(os.Stderr, "FICLI_API_KEY is required")
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
