package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/bootstrap"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/evals"
	"github.com/usehivy/hivy/internal/tasks"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "employee-eval: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var suitePath string
	var modelsCSV string
	var runs int
	var parallel int
	var apiURL string
	var outDir string
	var judgeModel string
	var verbose bool
	flag.StringVar(&suitePath, "suite", "evals/employee-delegation-v1.yaml", "eval suite YAML path")
	flag.StringVar(&modelsCSV, "models", "", "comma-separated model ids; defaults to suite models")
	flag.IntVar(&runs, "runs", 1, "number of runs per model/case")
	flag.IntVar(&parallel, "parallel", 1, "maximum concurrent trials")
	flag.StringVar(&apiURL, "api-url", "http://localhost:8080", "local control-plane API URL")
	flag.StringVar(&outDir, "out", "", "artifact output directory")
	flag.StringVar(&judgeModel, "judge-model", evals.DefaultJudgeModel, "model used for nondeterministic eval judgement")
	flag.BoolVar(&verbose, "verbose", true, "log detailed eval setup, runtime events, tool calls, judgement, and model usage to stdout")
	flag.Parse()

	suite, err := evals.LoadSuite(suitePath)
	if err != nil {
		return err
	}
	if outDir == "" {
		stamp := time.Now().UTC().Format("20060102T150405Z") + "-" + uuid.NewString()[:8]
		outDir = filepath.Join("tmp", "evals", "runs", stamp)
	}
	ctx := context.Background()
	deps, err := bootstrap.New(ctx)
	if err != nil {
		return err
	}
	defer deps.Close(ctx)
	redisOpt, err := deps.Config.AsynqRedisOpt()
	if err != nil {
		return fmt.Errorf("employee-eval: %w", err)
	}
	enqueuer := enqueue.NewClient(redisOpt)
	defer enqueuer.Close()
	if deps.Orchestrator != nil {
		deps.Orchestrator.SetWarmPoolReconciler(func(ctx context.Context, providerID, mode string) error {
			return tasks.EnqueueSandboxWarmPoolReconcile(ctx, enqueuer, providerID, mode)
		})
		tasks.EnqueueConfiguredWarmPoolReconciles(ctx, enqueuer, deps.Orchestrator)
	}

	opts := evals.RunOptions{
		SuitePath:  suitePath,
		Models:     splitCSV(modelsCSV),
		Runs:       runs,
		Parallel:   parallel,
		APIURL:     apiURL,
		OutDir:     outDir,
		JudgeModel: judgeModel,
		Verbose:    verbose,
	}
	summary, runErr := evals.NewRunner(deps).Run(ctx, suite, opts)
	if summary != nil {
		if err := evals.WriteArtifacts(outDir, suite, summary, deps.DB); err != nil && runErr == nil {
			runErr = err
		}
		fmt.Printf("eval artifacts: %s\n", outDir)
		printSummary(summary)
	}
	return runErr
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func printSummary(summary *evals.Summary) {
	fmt.Printf("overall pass rate: %.1f%% (%d/%d)\n",
		summary.Overall.PassRate,
		summary.Overall.Passed,
		summary.Overall.TotalCases,
	)
	fmt.Printf("overall delegation %.1f%%, correct specialist %.1f%%, false delegation %.1f%%, clarify %.1f%%, direct %.1f%%\n",
		summary.Overall.DelegationAccuracy,
		summary.Overall.CorrectSpecialistRate,
		summary.Overall.FalseDelegationRate,
		summary.Overall.ClarificationAccuracy,
		summary.Overall.DirectAnswerAccuracy,
	)
	fmt.Printf("overall avg decision %.1fs, avg cost $%.6f, avg credits %.2f\n",
		summary.Overall.AverageDecisionSeconds,
		summary.Overall.AverageCostUSD,
		summary.Overall.AverageCreditsDebited,
	)

	fmt.Println()
	fmt.Println("models:")
	for _, model := range summary.Models {
		fmt.Printf("- %s: %.1f%% (%d/%d), delegation %.1f%%, specialist %.1f%%, false delegation %.1f%%, avg %.1fs, avg $%.6f, avg credits %.2f\n",
			model.Model,
			model.PassRate,
			model.Passed,
			model.TotalCases,
			model.DelegationAccuracy,
			model.CorrectSpecialistRate,
			model.FalseDelegationRate,
			model.AverageDecisionSeconds,
			model.AverageCostUSD,
			model.AverageCreditsDebited,
		)
	}

	fmt.Println()
	fmt.Println("case results:")
	for _, run := range summary.Runs {
		status := "PASS"
		if !run.Passed {
			status = "FAIL"
		}
		fmt.Printf("- %s %-5s %s run %d: %s expected=%s/%s actual=%s/%s gen=%d tokens=%d/%d reasoning=%d cost=$%.6f credits=%d decision=%.1fs\n",
			run.Key.Model,
			status,
			run.Key.CaseID,
			run.Key.RunIndex,
			run.Reason,
			run.Case.ExpectedBehavior,
			emptyDash(run.Case.ExpectedSpecialist),
			emptyDash(run.Decision.Behavior),
			emptyDash(run.Decision.SpecialistSlug),
			run.Metrics.GenerationCount,
			run.Metrics.InputTokens,
			run.Metrics.OutputTokens,
			run.Metrics.ReasoningTokens,
			run.Metrics.CostUSD,
			run.Metrics.CreditsDebited,
			float64(run.Metrics.TimeToDecisionMS)/1000,
		)
		if !run.Passed && run.Error != "" {
			fmt.Printf("  error: %s\n", run.Error)
		}
	}

	failures := failedRuns(summary.Runs)
	fmt.Println()
	fmt.Printf("failures: %d\n", len(failures))
	for _, run := range failures {
		fmt.Printf("- %s / %s / run %d: %s; expected %s/%s, got %s/%s\n",
			run.Key.Model,
			run.Key.CaseID,
			run.Key.RunIndex,
			run.Reason,
			run.Case.ExpectedBehavior,
			emptyDash(run.Case.ExpectedSpecialist),
			emptyDash(run.Decision.Behavior),
			emptyDash(run.Decision.SpecialistSlug),
		)
		if run.Error != "" {
			fmt.Printf("  error: %s\n", run.Error)
		}
	}

	fmt.Println()
	printTopRuns("highest cost runs", summary.Runs, func(a, b evals.TrialResult) bool {
		return a.Metrics.CostUSD > b.Metrics.CostUSD
	})
	printTopRuns("slowest decision runs", summary.Runs, func(a, b evals.TrialResult) bool {
		return a.Metrics.TimeToDecisionMS > b.Metrics.TimeToDecisionMS
	})
}

func failedRuns(runs []evals.TrialResult) []evals.TrialResult {
	out := []evals.TrialResult{}
	for _, run := range runs {
		if !run.Passed {
			out = append(out, run)
		}
	}
	return out
}

func printTopRuns(title string, runs []evals.TrialResult, less func(a, b evals.TrialResult) bool) {
	ordered := append([]evals.TrialResult(nil), runs...)
	sort.SliceStable(ordered, func(i, j int) bool { return less(ordered[i], ordered[j]) })
	if len(ordered) > 5 {
		ordered = ordered[:5]
	}
	fmt.Printf("%s:\n", title)
	for _, run := range ordered {
		fmt.Printf("- %s / %s: cost=$%.6f credits=%d decision=%.1fs gen=%d tokens=%d/%d reasoning=%d\n",
			run.Key.Model,
			run.Key.CaseID,
			run.Metrics.CostUSD,
			run.Metrics.CreditsDebited,
			float64(run.Metrics.TimeToDecisionMS)/1000,
			run.Metrics.GenerationCount,
			run.Metrics.InputTokens,
			run.Metrics.OutputTokens,
			run.Metrics.ReasoningTokens,
		)
	}
}

func emptyDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}
