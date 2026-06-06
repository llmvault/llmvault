package evals

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/bootstrap"
	"github.com/usehivy/hivy/internal/registry"
)

type Runner struct {
	deps     *bootstrap.Deps
	client   *http.Client
	judge    *Judge
	reporter *ConsoleReporter
}

func NewRunner(deps *bootstrap.Deps) *Runner {
	return &Runner{deps: deps, client: &http.Client{Timeout: 30 * time.Second}}
}

func (r *Runner) Run(ctx context.Context, suite *Suite, opts RunOptions) (*Summary, error) {
	models := opts.Models
	if len(models) == 0 {
		models = suite.Models
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("at least one model is required")
	}
	for _, modelID := range models {
		if err := registry.Global().ValidateCanonicalModel(modelID); err != nil {
			return nil, fmt.Errorf("model %q: %w", modelID, err)
		}
	}
	if err := registry.Global().ValidateCanonicalModel(judgeModel(opts.JudgeModel)); err != nil {
		return nil, fmt.Errorf("judge model %q: %w", judgeModel(opts.JudgeModel), err)
	}
	runs := opts.Runs
	if runs <= 0 {
		runs = 1
	}
	parallel := opts.Parallel
	if parallel <= 0 {
		parallel = 1
	}
	apiURL := strings.TrimRight(opts.APIURL, "/")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}
	r.judge = NewJudge(opts.JudgeModel)
	r.reporter = NewConsoleReporter(opts.Verbose, nil)
	r.reporter.Run("eval run started",
		"suite", suite.ID,
		"suite_path", opts.SuitePath,
		"models", strings.Join(models, ","),
		"cases", len(suite.Cases),
		"runs_per_case", runs,
		"parallel", parallel,
		"api_url", apiURL,
		"judge_model", r.judge.model,
		"out_dir", opts.OutDir,
	)

	jobs := buildJobs(suite, models, runs)
	results := make([]TrialResult, len(jobs))
	work := make(chan int)
	var wg sync.WaitGroup
	for i := 0; i < parallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range work {
				result, _ := r.runTrial(ctx, suite, jobs[index], apiURL)
				results[index] = result
			}
		}()
	}
	for index := range jobs {
		work <- index
	}
	close(work)
	wg.Wait()
	return BuildSummary(suite.ID, results), nil
}

func buildJobs(suite *Suite, models []string, runs int) []TrialKey {
	jobs := []TrialKey{}
	for _, modelID := range models {
		for _, c := range suite.Cases {
			for i := 1; i <= runs; i++ {
				jobs = append(jobs, TrialKey{
					SuiteID:  suite.ID,
					Model:    modelID,
					CaseID:   c.ID,
					RunIndex: i,
				})
			}
		}
	}
	return jobs
}

func (r *Runner) runTrial(ctx context.Context, suite *Suite, key TrialKey, apiURL string) (TrialResult, error) {
	c := suiteCase(suite, key.CaseID)
	setupCtx, setupCancel := context.WithTimeout(ctx, 2*time.Minute)
	defer setupCancel()

	result := TrialResult{Key: key, Case: c, StartedAt: time.Now().UTC()}
	r.reporter.Trial(key, "eval trial setup started",
		"message", redactAndTrim(c.Message, 800),
		"expected_behavior", c.ExpectedBehavior,
		"expected_initial_behavior", c.ExpectedInitial,
		"expected_specialist", c.ExpectedSpecialist,
		"timeout_seconds", suite.TimeoutFor(c).Seconds(),
	)
	fixture, err := setupTrial(setupCtx, r.deps, suite, key, r.judge.model, r.reporter)
	result.Fixture = fixture
	defer r.cleanupTrial(result.Fixture)
	if err != nil {
		r.reporter.Trial(key, "eval trial setup failed", "error", err.Error())
		return failedResult(result, err), err
	}
	trialCtx, cancel := context.WithTimeout(ctx, suite.TimeoutFor(c))
	defer cancel()

	started := time.Now().UTC()
	result.StartedAt = started
	r.reporter.Trial(key, "eval sending first gateway message",
		"thread_id", fixture.ThreadID,
		"message_id", fixture.MessageID,
		"message", redactAndTrim(firstGatewayMessage(suite, key, c.Message), 1200),
	)
	gateway, err := r.sendGatewayMessage(trialCtx, apiURL, fixture, firstGatewayMessage(suite, key, c.Message))
	result.Gateway = gateway
	if err != nil {
		r.reporter.Trial(key, "eval gateway send failed", "error", err.Error())
		return failedResult(result, err), err
	}
	r.reporter.Trial(key, "eval gateway accepted first message",
		"event_id", gateway.EventID,
		"employee_session_id", gateway.EmployeeSessionID,
		"runtime_session_id", gateway.RuntimeSessionID,
		"runtime_stream_id", gateway.RuntimeStreamID,
		"runtime_trace_id", gateway.RuntimeTraceID,
		"runtime_turn_id", gateway.RuntimeTurnID,
	)
	ev, err := r.waitForDecision(trialCtx, suite.TimeoutFor(c), fixture, initialCase(c), apiURL, started)
	if err == nil && c.FollowUp != nil {
		r.reporter.Trial(key, "eval generating clarification follow-up",
			"judge_model", r.judge.model,
			"context", redactAndTrim(c.FollowUp.Context, 1000),
			"assistant_text", redactAndTrim(ev.Evidence.FinalText, 1000),
		)
		followUp, genErr := r.judge.GenerateFollowUp(trialCtx, proxyBaseURL(apiURL), fixture.JudgeProxyToken, c, ev.Evidence.FinalText)
		if genErr != nil {
			err = fmt.Errorf("generate clarification follow-up: %w", genErr)
		} else {
			r.reporter.Trial(key, "eval generated clarification follow-up",
				"follow_up", redactAndTrim(followUp, 1000),
			)
			followUpAt := time.Now().UTC()
			if _, sendErr := r.sendGatewayMessageWithID(trialCtx, apiURL, fixture, followUp, "msg:"+uuid.NewString()); sendErr != nil {
				err = sendErr
			} else {
				ev, err = r.waitForDecision(trialCtx, suite.TimeoutFor(c), fixture, c, apiURL, followUpAt)
			}
		}
	}
	result.Evidence = ev.Evidence
	if err != nil {
		r.reporter.Trial(key, "eval wait failed", "error", err.Error(), "last_reason", ev.Reason)
		result = failedResult(result, err)
		result.Decision = ev.Decision
		result.Metrics = r.metrics(context.Background(), fixture, started)
		result.Evidence = ev.Evidence
		return result, nil
	}
	result.Passed = ev.Passed
	result.Reason = ev.Reason
	result.Decision = ev.Decision
	result.EndedAt = time.Now().UTC()
	result.Metrics = r.metrics(ctx, fixture, started)
	r.reporter.Trial(key, "eval trial finished",
		"passed", result.Passed,
		"reason", result.Reason,
		"behavior", result.Decision.Behavior,
		"specialist", result.Decision.SpecialistSlug,
		"generation_count", result.Metrics.GenerationCount,
		"input_tokens", result.Metrics.InputTokens,
		"output_tokens", result.Metrics.OutputTokens,
		"reasoning_tokens", result.Metrics.ReasoningTokens,
		"cost_usd", fmt.Sprintf("%.8f", result.Metrics.CostUSD),
		"credits", result.Metrics.CreditsDebited,
	)
	if !result.Decision.DecidedAt.IsZero() {
		result.Metrics.TimeToDecisionMS = result.Decision.DecidedAt.Sub(started).Milliseconds()
	}
	return result, nil
}

type evaluatedEvidence struct {
	Evidence Evidence
	Passed   bool
	Reason   string
	Decision Decision
}

func (r *Runner) waitForDecision(ctx context.Context, timeout time.Duration, fixture TrialFixture, c Case, apiURL string, since time.Time) (evaluatedEvidence, error) {
	deadline := time.Now().Add(timeout)
	var last evaluatedEvidence
	for time.Now().Before(deadline) {
		evidence, err := r.loadEvidenceSince(ctx, fixture, since)
		if err != nil {
			return last, err
		}
		r.reporter.ObserveEvidence(fixture.Key, evidence)
		if generations, genErr := r.loadGenerationsSince(ctx, fixture, since); genErr == nil {
			r.reporter.ObserveGenerations(fixture.Key, generations, fixture.JudgeTokenJTI)
		} else {
			r.reporter.Trial(fixture.Key, "eval failed to load generation metadata", "error", genErr.Error())
		}
		var judgement *BehaviorJudgement
		if len(evidence.Tasks) == 0 && strings.TrimSpace(evidence.FinalText) != "" {
			var err error
			r.reporter.Trial(fixture.Key, "eval judge classify request",
				"judge_model", r.judge.model,
				"final_text", redactAndTrim(evidence.FinalText, 1000),
			)
			judgement, err = r.judge.ClassifyFinalText(ctx, proxyBaseURL(apiURL), fixture.JudgeProxyToken, c, evidence.FinalText)
			if err != nil {
				return last, fmt.Errorf("judge final response: %w", err)
			}
			r.reporter.Trial(fixture.Key, "eval judge classify response",
				"behavior", judgement.Behavior,
				"confidence", judgement.Confidence,
				"reason", redactAndTrim(judgement.Reason, 600),
				"judge_model", judgement.Model,
			)
		}
		passed, reason, decision := GradeCaseWithJudgement(c, evidence, judgement)
		last = evaluatedEvidence{Evidence: evidence, Passed: passed, Reason: reason, Decision: decision}
		r.reporter.Trial(fixture.Key, "eval grade checkpoint",
			"passed", passed,
			"reason", reason,
			"behavior", decision.Behavior,
			"specialist", decision.SpecialistSlug,
			"final_text_present", strings.TrimSpace(evidence.FinalText) != "",
			"tasks", len(evidence.Tasks),
			"tool_calls", len(evidence.ToolCalls),
		)
		if IsTerminal(c, evidence) && !needsMoreObservation(c, evidence) {
			return last, nil
		}
		time.Sleep(2 * time.Second)
	}
	return last, fmt.Errorf("trial timed out after %s: %s", timeout, last.Reason)
}

func (r *Runner) sendGatewayMessage(ctx context.Context, apiURL string, fixture TrialFixture, message string) (GatewayResponse, error) {
	return r.sendGatewayMessageWithID(ctx, apiURL, fixture, message, fixture.MessageID)
}

func (r *Runner) sendGatewayMessageWithID(ctx context.Context, apiURL string, fixture TrialFixture, message, messageID string) (GatewayResponse, error) {
	body, _ := json.Marshal(map[string]any{
		"markdown":    message,
		"thread_id":   fixture.ThreadID,
		"message_id":  messageID,
		"sender_id":   "eval-user",
		"sender_name": "Eval User",
	})
	url := fmt.Sprintf("%s/incoming/gateways/http/%s", apiURL, fixture.RouteID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return GatewayResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return GatewayResponse{}, fmt.Errorf("post gateway message: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return GatewayResponse{}, fmt.Errorf("read gateway response: %w", err)
	}
	var out GatewayResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, fmt.Errorf("decode gateway response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, fmt.Errorf("gateway returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return out, nil
}
