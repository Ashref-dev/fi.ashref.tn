package review

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"fi-cli/internal/config"
	"fi-cli/internal/llm"
	"fi-cli/internal/repo"
	"fi-cli/internal/tools"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3"
	"go.uber.org/zap"
)

var identifierPattern = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]{2,}`)

type Reviewer struct {
	client        llm.Client
	logger        *zap.Logger
	cfg           config.Config
	refsTool      *tools.GitRefsTool
	mergeBaseTool *tools.GitMergeBaseTool
	changedTool   *tools.GitChangedFilesTool
	diffTool      *tools.GitDiffHunksTool
	fileTool      *tools.GitFileAtRefTool
	blameTool     *tools.GitBlameLinesTool
	logTool       *tools.GitLogRangeTool
	grepTool      *tools.GrepTool
	treeTool      *tools.ListTreeTool
}

type reviewCandidate struct {
	ChangedFile tools.GitChangedFile
	Diff        tools.GitDiffHunksOutput
	Risk        int
}

type reviewContext struct {
	Candidate    reviewCandidate
	HeadFile     tools.GitFileAtRefOutput
	BaseFile     tools.GitFileAtRefOutput
	GrepPreview  string
	BlamePreview string
	LogPreview   string
	GlobalRules  string
	PathRules    string
	TreeSnapshot string
	RepoSummary  string
	Uncertainty  []string
	FileReview   fileReviewResponse
}

func NewReviewer(client llm.Client, logger *zap.Logger, cfg config.Config) *Reviewer {
	return &Reviewer{
		client:        client,
		logger:        logger,
		cfg:           cfg,
		refsTool:      tools.NewGitRefsTool(),
		mergeBaseTool: tools.NewGitMergeBaseTool(),
		changedTool:   tools.NewGitChangedFilesTool(),
		diffTool:      tools.NewGitDiffHunksTool(),
		fileTool:      tools.NewGitFileAtRefTool(),
		blameTool:     tools.NewGitBlameLinesTool(),
		logTool:       tools.NewGitLogRangeTool(),
		grepTool:      tools.NewGrepTool(),
		treeTool:      tools.NewListTreeTool(),
	}
}

func (r *Reviewer) Run(ctx context.Context, params Params, repoRoot string, repoCtx repo.RepoContext) (Result, error) {
	started := time.Now()
	result := Result{
		RunID:          uuid.NewString(),
		StartedAt:      started,
		RepoRoot:       repoRoot,
		Model:          r.cfg.Model,
		StageTimingsMs: DefaultStageTimings(),
	}

	globalRules, err := loadReviewInstructions(repoRoot, r.cfg.ReviewInstructionsFile, ".fi/review.md")
	if err != nil {
		return result, err
	}
	pathRules, err := loadPathRules(repoRoot, r.cfg.ReviewPathRulesFile, ".fi/review-paths.yaml")
	if err != nil {
		return result, err
	}

	refStart := time.Now()
	baseRef, headRef, err := r.resolveRefs(ctx, repoRoot, params)
	result.StageTimingsMs[StageRefResolution] = durationMs(time.Since(refStart))
	if err != nil {
		return result, err
	}
	result.BaseRef = baseRef
	result.HeadRef = headRef

	mergeBaseStart := time.Now()
	mergeBase, err := r.resolveMergeBase(ctx, repoRoot, baseRef, headRef)
	result.StageTimingsMs[StageMergeBase] = durationMs(time.Since(mergeBaseStart))
	if err != nil {
		return result, err
	}
	result.MergeBase = mergeBase

	changedStart := time.Now()
	changedFiles, err := r.readChangedFiles(ctx, repoRoot, mergeBase, headRef, params.WorkingTree)
	result.StageTimingsMs[StageChangedFilesScan] = durationMs(time.Since(changedStart))
	if err != nil {
		return result, err
	}
	result.Coverage.TotalChanged = len(changedFiles)
	result.Coverage.WorkingTreeIncluded = params.WorkingTree

	treeSnapshot := r.loadTreeSnapshot(ctx, repoRoot)

	diffStart := time.Now()
	candidates, skippedFiles, coverage, err := r.loadReviewCandidates(ctx, repoRoot, mergeBase, headRef, params.WorkingTree, changedFiles)
	result.StageTimingsMs[StageDiffContextBuild] = durationMs(time.Since(diffStart))
	if err != nil {
		return result, err
	}
	result.SkippedFiles = append(result.SkippedFiles, skippedFiles...)
	result.Coverage.BinaryCount += coverage.BinaryCount
	result.Coverage.OverSizedCount += coverage.OverSizedCount

	triageStart := time.Now()
	sortReviewCandidates(candidates)
	result.StageTimingsMs[StageTriage] = durationMs(time.Since(triageStart))

	selected, skippedByLimit := selectReviewCandidates(candidates, r.cfg.ReviewMaxFiles)
	result.SkippedFiles = append(result.SkippedFiles, skippedByLimit...)

	deepReviewStart := time.Now()
	contexts, modelWait, skippedDeep, err := r.deepReview(ctx, repoRoot, repoCtx, mergeBase, headRef, params.WorkingTree, treeSnapshot, globalRules, pathRules, selected)
	result.StageTimingsMs[StageDeepReview] = durationMs(time.Since(deepReviewStart))
	result.StageTimingsMs[StageModelWaitTotal] = modelWait
	if err != nil {
		return result, err
	}
	result.SkippedFiles = append(result.SkippedFiles, skippedDeep...)

	aggregationStart := time.Now()
	r.aggregate(&result, contexts)
	result.StageTimingsMs[StageAggregation] = durationMs(time.Since(aggregationStart))

	result.Coverage.ReviewedCount = len(result.ReviewedFiles)
	result.Coverage.SkippedCount = len(result.SkippedFiles)
	if len(result.ReviewedFiles) == 0 && result.Coverage.TotalChanged > 0 {
		return result, errors.New("review incomplete: no files were reviewed")
	}

	result.FinishedAt = time.Now()
	result.StageTimingsMs[StageTotalRunDuration] = durationMs(result.FinishedAt.Sub(started))
	return result, nil
}

func (r *Reviewer) resolveRefs(ctx context.Context, repoRoot string, params Params) (string, string, error) {
	headRef := strings.TrimSpace(params.HeadRef)
	if headRef == "" {
		headRef = "HEAD"
	}
	if params.WorkingTree && headRef != "HEAD" {
		return "", "", errors.New("--working-tree requires --head HEAD")
	}

	baseRef := strings.TrimSpace(params.BaseRef)
	if baseRef != "" {
		return baseRef, headRef, nil
	}

	candidates := []string{"origin/HEAD", "origin/main", "main", "origin/master", "master"}
	output, err := r.readGitRefs(ctx, repoRoot, candidates)
	if err != nil {
		return "", "", err
	}
	tried := make([]string, 0, len(output.Candidates))
	for _, candidate := range output.Candidates {
		tried = append(tried, candidate.Ref)
		if candidate.Exists {
			resolved := strings.TrimSpace(candidate.ResolvedRef)
			if resolved != "" {
				return resolved, headRef, nil
			}
			return candidate.Ref, headRef, nil
		}
	}
	return "", "", fmt.Errorf("failed to auto-detect base ref; tried: %s. Pass --base explicitly", strings.Join(tried, ", "))
}

func (r *Reviewer) resolveMergeBase(ctx context.Context, repoRoot string, baseRef string, headRef string) (string, error) {
	output, err := r.readMergeBase(ctx, repoRoot, baseRef, headRef)
	if err != nil {
		return "", err
	}
	return output.MergeBase, nil
}

func (r *Reviewer) readChangedFiles(ctx context.Context, repoRoot string, baseRef string, headRef string, workingTree bool) ([]tools.GitChangedFile, error) {
	output, err := r.readGitChangedFiles(ctx, repoRoot, baseRef, headRef, workingTree)
	if err != nil {
		return nil, err
	}
	return output.ChangedFiles, nil
}

func (r *Reviewer) loadTreeSnapshot(ctx context.Context, repoRoot string) string {
	payload, err := r.executeTool(ctx, r.treeTool, repoRoot, map[string]any{
		"path":        ".",
		"max_depth":   2,
		"max_entries": 200,
	}, r.cfg.ToolLimits.ContextMaxBytes, 0)
	if err != nil {
		return ""
	}
	output, ok := payload.(tools.ListTreeOutputAlias)
	if ok {
		return strings.Join(output.Lines, "\n")
	}
	// Fall back to preview if type alias is unavailable.
	if result, ok := payload.(map[string]any); ok {
		if lines, ok := result["lines"].([]string); ok {
			return strings.Join(lines, "\n")
		}
	}
	return ""
}

func (r *Reviewer) loadReviewCandidates(ctx context.Context, repoRoot string, mergeBase string, headRef string, workingTree bool, changedFiles []tools.GitChangedFile) ([]reviewCandidate, []string, Coverage, error) {
	parallelism := maxInt(1, r.cfg.ToolParallelism)
	type candidateResult struct {
		candidate reviewCandidate
		skip      string
		binary    bool
		oversized bool
		err       error
	}
	results := make([]candidateResult, len(changedFiles))
	sem := make(chan struct{}, parallelism)
	var wg sync.WaitGroup
	for i, changedFile := range changedFiles {
		i := i
		changedFile := changedFile
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if changedFile.Binary {
				results[i] = candidateResult{skip: fmt.Sprintf("%s (binary)", changedFile.Path), binary: true}
				return
			}
			diffOutput, err := r.readDiff(ctx, repoRoot, mergeBase, headRef, changedFile, workingTree)
			if err != nil {
				results[i] = candidateResult{err: err}
				return
			}
			if diffOutput.Truncated {
				results[i] = candidateResult{skip: fmt.Sprintf("%s (oversized diff)", changedFile.Path), oversized: true}
				return
			}
			results[i] = candidateResult{
				candidate: reviewCandidate{
					ChangedFile: changedFile,
					Diff:        diffOutput,
					Risk:        scoreReviewRisk(changedFile, diffOutput.Patch),
				},
			}
		}()
	}
	wg.Wait()

	candidates := make([]reviewCandidate, 0, len(results))
	skipped := []string{}
	coverage := Coverage{}
	for _, item := range results {
		if item.err != nil {
			return nil, nil, coverage, item.err
		}
		if item.binary {
			coverage.BinaryCount++
		}
		if item.oversized {
			coverage.OverSizedCount++
		}
		if item.skip != "" {
			skipped = append(skipped, item.skip)
			continue
		}
		candidates = append(candidates, item.candidate)
	}
	return candidates, skipped, coverage, nil
}

func (r *Reviewer) deepReview(
	ctx context.Context,
	repoRoot string,
	repoCtx repo.RepoContext,
	mergeBase string,
	headRef string,
	workingTree bool,
	treeSnapshot string,
	globalRules string,
	pathRules []pathRule,
	selected []reviewCandidate,
) ([]reviewContext, int64, []string, error) {
	parallelism := maxInt(1, r.cfg.ToolParallelism)
	contexts := make([]reviewContext, len(selected))
	skipped := make([]string, 0)
	var skippedMu sync.Mutex
	var modelWaitTotal int64
	var modelWaitMu sync.Mutex
	sem := make(chan struct{}, parallelism)
	var wg sync.WaitGroup
	var firstErr error
	var firstErrMu sync.Mutex

	for idx, candidate := range selected {
		idx := idx
		candidate := candidate
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			ctxValue, skip, waitMs, err := r.buildAndReviewFile(ctx, repoRoot, repoCtx, mergeBase, headRef, workingTree, treeSnapshot, globalRules, pathRules, candidate)
			modelWaitMu.Lock()
			modelWaitTotal += waitMs
			modelWaitMu.Unlock()
			if err != nil {
				firstErrMu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				firstErrMu.Unlock()
				return
			}
			if skip != "" {
				skippedMu.Lock()
				skipped = append(skipped, skip)
				skippedMu.Unlock()
				return
			}
			contexts[idx] = ctxValue
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return nil, modelWaitTotal, nil, firstErr
	}

	out := make([]reviewContext, 0, len(contexts))
	for _, item := range contexts {
		if item.Candidate.ChangedFile.Path == "" {
			continue
		}
		out = append(out, item)
	}
	return out, modelWaitTotal, skipped, nil
}

func (r *Reviewer) buildAndReviewFile(
	ctx context.Context,
	repoRoot string,
	repoCtx repo.RepoContext,
	mergeBase string,
	headRef string,
	workingTree bool,
	treeSnapshot string,
	globalRules string,
	pathRules []pathRule,
	candidate reviewCandidate,
) (reviewContext, string, int64, error) {
	ctxValue := reviewContext{
		Candidate:    candidate,
		TreeSnapshot: treeSnapshot,
		RepoSummary:  repoCtx.Summary(),
		GlobalRules:  globalRules,
		PathRules:    resolvePathInstructions(pathRules, candidate.ChangedFile.Path),
	}

	basePath := candidate.ChangedFile.Path
	if candidate.ChangedFile.OldPath != "" {
		basePath = candidate.ChangedFile.OldPath
	}

	var (
		mu         sync.Mutex
		wg         sync.WaitGroup
		firstErr   error
		skipReason string
	)
	setErr := func(err error) {
		if err == nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if firstErr == nil {
			firstErr = err
		}
	}
	setSkip := func(reason string) {
		if strings.TrimSpace(reason) == "" {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if skipReason == "" {
			skipReason = reason
		}
	}
	appendUncertainty := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		mu.Lock()
		ctxValue.Uncertainty = append(ctxValue.Uncertainty, value)
		mu.Unlock()
	}

	if candidate.ChangedFile.Status != "D" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			headOutput, err := r.readFileAtRef(ctx, repoRoot, headRef, candidate.ChangedFile.Path, workingTree)
			if err != nil {
				setErr(err)
				return
			}
			if headOutput.Truncated {
				setSkip(fmt.Sprintf("%s (oversized head context)", candidate.ChangedFile.Path))
				return
			}
			mu.Lock()
			ctxValue.HeadFile = headOutput
			mu.Unlock()
		}()
	}

	if candidate.ChangedFile.Status != "A" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			baseOutput, err := r.readFileAtRef(ctx, repoRoot, mergeBase, basePath, false)
			if err != nil {
				setErr(err)
				return
			}
			if baseOutput.Truncated {
				setSkip(fmt.Sprintf("%s (oversized base context)", candidate.ChangedFile.Path))
				return
			}
			mu.Lock()
			ctxValue.BaseFile = baseOutput
			mu.Unlock()
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		grepPreview, err := r.loadGrepEvidence(ctx, repoRoot, candidate)
		if err != nil {
			appendUncertainty("grep evidence unavailable: " + err.Error())
			return
		}
		mu.Lock()
		ctxValue.GrepPreview = grepPreview
		mu.Unlock()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		blamePreview, err := r.loadBlameEvidence(ctx, repoRoot, candidate, headRef, workingTree)
		if err != nil {
			appendUncertainty("blame unavailable: " + err.Error())
			return
		}
		mu.Lock()
		ctxValue.BlamePreview = blamePreview
		mu.Unlock()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		logPreview, err := r.loadLogEvidence(ctx, repoRoot, mergeBase, headRef, candidate)
		if err != nil {
			appendUncertainty("log range unavailable: " + err.Error())
			return
		}
		mu.Lock()
		ctxValue.LogPreview = logPreview
		mu.Unlock()
	}()

	wg.Wait()
	if firstErr != nil {
		return reviewContext{}, "", 0, firstErr
	}
	if skipReason != "" {
		return reviewContext{}, skipReason, 0, nil
	}

	promptInput := reviewPromptInput{
		Path:               candidate.ChangedFile.Path,
		Status:             candidate.ChangedFile.Status,
		Additions:          candidate.ChangedFile.Additions,
		Deletions:          candidate.ChangedFile.Deletions,
		RepoSummary:        ctxValue.RepoSummary,
		TreeSnapshot:       ctxValue.TreeSnapshot,
		GlobalInstructions: ctxValue.GlobalRules,
		PathInstructions:   ctxValue.PathRules,
		DiffPatch:          candidate.Diff.Patch,
		HeadContext:        ctxValue.HeadFile.Content,
		BaseContext:        ctxValue.BaseFile.Content,
		GrepEvidence:       ctxValue.GrepPreview,
		BlameEvidence:      ctxValue.BlamePreview,
		LogEvidence:        ctxValue.LogPreview,
	}

	modelStart := time.Now()
	resp, err := r.client.Create(ctx, llm.Request{
		Model: r.cfg.Model,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(reviewSystemPrompt()),
			openai.DeveloperMessage(reviewDeveloperPrompt(promptInput)),
			openai.UserMessage("Return the JSON review object now."),
		},
	})
	modelWait := durationMs(time.Since(modelStart))
	if err != nil {
		return reviewContext{}, "", modelWait, err
	}

	fileReview, err := parseFileReviewResponse(resp.Content, candidate.ChangedFile.Path)
	if err != nil {
		return reviewContext{}, "", modelWait, err
	}
	ctxValue.FileReview = fileReview
	if len(fileReview.Uncertainty) > 0 {
		ctxValue.Uncertainty = append(ctxValue.Uncertainty, fileReview.Uncertainty...)
	}
	return ctxValue, "", modelWait, nil
}

func (r *Reviewer) aggregate(result *Result, contexts []reviewContext) {
	findings := make([]Finding, 0)
	strengths := []string{}
	uncertainty := append([]string(nil), result.Coverage.Uncertainty...)
	reviewedFiles := make([]string, 0, len(contexts))
	for _, ctxValue := range contexts {
		reviewedFiles = append(reviewedFiles, ctxValue.Candidate.ChangedFile.Path)
		for _, finding := range ctxValue.FileReview.Findings {
			findings = append(findings, NormalizeFinding(finding, ctxValue.Candidate.ChangedFile.Path))
		}
		strengths = append(strengths, ctxValue.FileReview.Strengths...)
		uncertainty = append(uncertainty, ctxValue.Uncertainty...)
	}
	SortFindings(findings)
	if len(findings) > r.cfg.ReviewMaxFindings {
		findings = findings[:r.cfg.ReviewMaxFindings]
		result.Coverage.Notes = append(result.Coverage.Notes, fmt.Sprintf("Trimmed findings to review_max_findings=%d", r.cfg.ReviewMaxFindings))
	}
	result.Findings = findings
	result.Strengths = uniqStringsKeepOrder(strengths)
	if len(result.Strengths) > 5 {
		result.Strengths = result.Strengths[:5]
	}
	result.ReviewedFiles = uniqStringsKeepOrder(reviewedFiles)
	sort.Strings(result.ReviewedFiles)
	sort.Strings(result.SkippedFiles)
	result.BlockerCount = CountBlockers(findings)
	result.Score5 = ScoreFindings(findings)
	result.MergeReady = result.BlockerCount == 0
	result.Coverage.Uncertainty = uniqStringsKeepOrder(uncertainty)
	result.Summary = buildSummary(findings, result.MergeReady)
}

func buildSummary(findings []Finding, mergeReady bool) string {
	if len(findings) == 0 {
		return "No material issues found in reviewed files."
	}
	counts := map[string]int{"blocker": 0, "high": 0, "medium": 0, "low": 0}
	for _, finding := range findings {
		counts[normalizeSeverity(finding.Severity)]++
	}
	if !mergeReady {
		return fmt.Sprintf("Found %d blocker(s), %d high, %d medium, and %d low issues.", counts["blocker"], counts["high"], counts["medium"], counts["low"])
	}
	return fmt.Sprintf("Found %d high, %d medium, and %d low issues. No blockers detected.", counts["high"], counts["medium"], counts["low"])
}

func sortReviewCandidates(candidates []reviewCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Risk != candidates[j].Risk {
			return candidates[i].Risk > candidates[j].Risk
		}
		return candidates[i].ChangedFile.Path < candidates[j].ChangedFile.Path
	})
}

func selectReviewCandidates(candidates []reviewCandidate, limit int) ([]reviewCandidate, []string) {
	if limit <= 0 || len(candidates) <= limit {
		return candidates, nil
	}
	skipped := make([]string, 0, len(candidates)-limit)
	for _, candidate := range candidates[limit:] {
		skipped = append(skipped, fmt.Sprintf("%s (skipped by review_max_files)", candidate.ChangedFile.Path))
	}
	return candidates[:limit], skipped
}

func scoreReviewRisk(file tools.GitChangedFile, patch string) int {
	score := file.Additions + file.Deletions
	switch file.Status {
	case "R", "D":
		score += 15
	case "A":
		score += 10
	}
	lowerPath := strings.ToLower(file.Path)
	for _, keyword := range []string{"auth", "security", "billing", "payment", "migration", "schema", "config", "workflow", "deploy", "api", "handler", "router"} {
		if strings.Contains(lowerPath, keyword) {
			score += 20
		}
	}
	if strings.Contains(lowerPath, "_test.") || strings.Contains(lowerPath, "/test") || strings.Contains(lowerPath, "/spec") {
		score -= 10
	}
	score += strings.Count(patch, "@@") * 3
	return score
}

func (r *Reviewer) loadGrepEvidence(ctx context.Context, repoRoot string, candidate reviewCandidate) (string, error) {
	pattern := buildReviewGrepPattern(candidate.Diff.Patch, candidate.ChangedFile.Path)
	output, err := r.executeTool(ctx, r.grepTool, repoRoot, map[string]any{
		"pattern":        pattern,
		"case_sensitive": false,
		"max_results":    10,
	}, r.cfg.ToolLimits.GrepMaxBytes, r.cfg.ToolLimits.GrepMaxResults)
	if err != nil {
		return "", err
	}
	if grepOutput, ok := output.(tools.GrepOutputAlias); ok {
		return strings.Join(grepOutput.Matches, "\n"), nil
	}
	return "", nil
}

func (r *Reviewer) loadBlameEvidence(ctx context.Context, repoRoot string, candidate reviewCandidate, headRef string, workingTree bool) (string, error) {
	if candidate.ChangedFile.Status == "A" {
		return "", nil
	}
	startLine, endLine, ok := selectBlameRange(candidate.Diff.Hunks)
	if !ok {
		return "", nil
	}
	ref := headRef
	if workingTree {
		ref = "HEAD"
	}
	output, err := r.executeTool(ctx, r.blameTool, repoRoot, map[string]any{
		"ref":        ref,
		"path":       candidate.ChangedFile.Path,
		"start_line": startLine,
		"end_line":   endLine,
	}, r.cfg.ToolLimits.ContextMaxBytes, 0)
	if err != nil {
		return "", err
	}
	if blameOutput, ok := output.(tools.GitBlameOutput); ok {
		lines := make([]string, 0, len(blameOutput.Lines))
		for _, line := range blameOutput.Lines {
			lines = append(lines, fmt.Sprintf("%d %s %s", line.Line, trimReviewSHA(line.Commit), line.Summary))
		}
		return strings.Join(lines, "\n"), nil
	}
	return "", nil
}

func (r *Reviewer) loadLogEvidence(ctx context.Context, repoRoot string, mergeBase string, headRef string, candidate reviewCandidate) (string, error) {
	output, err := r.executeTool(ctx, r.logTool, repoRoot, map[string]any{
		"base_ref": mergeBase,
		"head_ref": headRef,
		"path":     candidate.ChangedFile.Path,
		"limit":    5,
	}, r.cfg.ToolLimits.ContextMaxBytes, 0)
	if err != nil {
		return "", err
	}
	if logOutput, ok := output.(tools.GitLogRangeOutput); ok {
		lines := make([]string, 0, len(logOutput.Entries))
		for _, entry := range logOutput.Entries {
			lines = append(lines, trimReviewSHA(entry.Commit)+" "+entry.Subject)
		}
		return strings.Join(lines, "\n"), nil
	}
	return "", nil
}

func (r *Reviewer) readGitRefs(ctx context.Context, repoRoot string, candidates []string) (tools.GitRefsOutput, error) {
	output, err := r.executeTool(ctx, r.refsTool, repoRoot, map[string]any{"candidates": candidates}, r.cfg.ToolLimits.ContextMaxBytes, 0)
	if err != nil {
		return tools.GitRefsOutput{}, err
	}
	typed, ok := output.(tools.GitRefsOutput)
	if !ok {
		return tools.GitRefsOutput{}, errors.New("unexpected git_refs output")
	}
	return typed, nil
}

func (r *Reviewer) readMergeBase(ctx context.Context, repoRoot string, baseRef string, headRef string) (tools.GitMergeBaseOutput, error) {
	output, err := r.executeTool(ctx, r.mergeBaseTool, repoRoot, map[string]any{"base_ref": baseRef, "head_ref": headRef}, r.cfg.ToolLimits.ContextMaxBytes, 0)
	if err != nil {
		return tools.GitMergeBaseOutput{}, err
	}
	typed, ok := output.(tools.GitMergeBaseOutput)
	if !ok {
		return tools.GitMergeBaseOutput{}, errors.New("unexpected git_merge_base output")
	}
	return typed, nil
}

func (r *Reviewer) readGitChangedFiles(ctx context.Context, repoRoot string, baseRef string, headRef string, workingTree bool) (tools.GitChangedFilesOutput, error) {
	output, err := r.executeTool(ctx, r.changedTool, repoRoot, map[string]any{"base_ref": baseRef, "head_ref": headRef, "working_tree": workingTree}, r.cfg.ToolLimits.ContextMaxBytes, 0)
	if err != nil {
		return tools.GitChangedFilesOutput{}, err
	}
	typed, ok := output.(tools.GitChangedFilesOutput)
	if !ok {
		return tools.GitChangedFilesOutput{}, errors.New("unexpected git_changed_files output")
	}
	return typed, nil
}

func (r *Reviewer) readDiff(ctx context.Context, repoRoot string, mergeBase string, headRef string, changedFile tools.GitChangedFile, workingTree bool) (tools.GitDiffHunksOutput, error) {
	output, err := r.executeTool(ctx, r.diffTool, repoRoot, map[string]any{
		"base_ref":     mergeBase,
		"head_ref":     headRef,
		"path":         changedFile.Path,
		"old_path":     changedFile.OldPath,
		"working_tree": workingTree,
	}, r.cfg.ToolLimits.ContextMaxBytes, 0)
	if err != nil {
		return tools.GitDiffHunksOutput{}, err
	}
	typed, ok := output.(tools.GitDiffHunksOutput)
	if !ok {
		return tools.GitDiffHunksOutput{}, errors.New("unexpected git_diff_hunks output")
	}
	return typed, nil
}

func (r *Reviewer) readFileAtRef(ctx context.Context, repoRoot string, ref string, path string, workingTree bool) (tools.GitFileAtRefOutput, error) {
	output, err := r.executeTool(ctx, r.fileTool, repoRoot, map[string]any{
		"ref":          ref,
		"path":         path,
		"working_tree": workingTree,
	}, r.cfg.ToolLimits.MaxFileBytes*2, 0)
	if err != nil {
		return tools.GitFileAtRefOutput{}, err
	}
	typed, ok := output.(tools.GitFileAtRefOutput)
	if !ok {
		return tools.GitFileAtRefOutput{}, errors.New("unexpected git_file_at_ref output")
	}
	return typed, nil
}

func (r *Reviewer) executeTool(ctx context.Context, tool tools.Tool, repoRoot string, input any, maxBytes int, maxResults int) (any, error) {
	args, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	result, err := tool.Execute(ctx, args, tools.Meta{
		RepoRoot:           repoRoot,
		ToolTimeoutSeconds: r.cfg.ToolTimeoutSeconds,
		MaxBytes:           maxBytes,
		MaxResults:         maxResults,
	})
	if err != nil {
		return nil, err
	}
	return result.Payload, nil
}

func parseFileReviewResponse(content string, fallbackPath string) (fileReviewResponse, error) {
	raw, err := extractJSONObject(content)
	if err != nil {
		return fileReviewResponse{}, err
	}
	var parsed fileReviewResponse
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return fileReviewResponse{}, err
	}
	for i := range parsed.Findings {
		parsed.Findings[i] = NormalizeFinding(parsed.Findings[i], fallbackPath)
	}
	return parsed, nil
}

func extractJSONObject(input string) (string, error) {
	start := strings.IndexByte(input, '{')
	if start == -1 {
		return "", errors.New("response did not contain JSON object")
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(input); i++ {
		ch := input[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return input[start : i+1], nil
			}
		}
	}
	return "", errors.New("response JSON object was incomplete")
}

func buildReviewGrepPattern(patch string, pathValue string) string {
	tokens := identifierPattern.FindAllString(patch, -1)
	stopWords := map[string]struct{}{
		"true": {}, "false": {}, "null": {}, "return": {}, "const": {}, "func": {}, "type": {},
		"package": {}, "import": {}, "class": {}, "interface": {}, "string": {}, "error": {},
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, 4)
	for _, token := range tokens {
		key := strings.ToLower(token)
		if _, blocked := stopWords[key]; blocked {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, regexp.QuoteMeta(token))
		if len(out) == 4 {
			break
		}
	}
	if len(out) == 0 {
		base := filepath.Base(pathValue)
		ext := filepath.Ext(base)
		base = strings.TrimSuffix(base, ext)
		if base == "" {
			base = pathValue
		}
		out = append(out, regexp.QuoteMeta(base))
	}
	return strings.Join(out, "|")
}

func selectBlameRange(hunks []tools.GitDiffHunk) (int, int, bool) {
	for _, hunk := range hunks {
		if len(hunk.AddedLines) == 0 && hunk.NewStart <= 0 {
			continue
		}
		start := hunk.NewStart
		if len(hunk.AddedLines) > 0 {
			start = hunk.AddedLines[0]
		}
		end := start + maxInt(hunk.NewLines, 1) - 1
		if end-start > 20 {
			end = start + 20
		}
		if start > 0 && end >= start {
			return start, end, true
		}
	}
	return 0, 0, false
}

func uniqStringsKeepOrder(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func trimReviewSHA(value string) string {
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}

func durationMs(d time.Duration) int64 {
	if d < 0 {
		return 0
	}
	return d.Milliseconds()
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}
