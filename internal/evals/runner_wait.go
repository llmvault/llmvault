package evals

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	evalPollInterval       = 2 * time.Second
	evalCheckpointInterval = 30 * time.Second
	evalNoActivityTimeout  = 45 * time.Second
)

type evaluatedEvidence struct {
	Evidence Evidence
	Passed   bool
	Reason   string
	Decision Decision
}

func (r *Runner) waitForDecision(ctx context.Context, timeout time.Duration, fixture TrialFixture, c Case, apiURL string, since time.Time) (evaluatedEvidence, error) {
	deadline := time.Now().Add(timeout)
	noActivityDeadline := time.Now().Add(minDuration(timeout, evalNoActivityTimeout))
	var last evaluatedEvidence
	var seenActivity bool
	var lastCheckpointAt time.Time
	var lastCheckpointSignature string

	for time.Now().Before(deadline) {
		evidence, err := r.loadEvidenceSince(ctx, fixture, since)
		if err != nil {
			return last, err
		}
		r.reporter.ObserveEvidence(fixture.Key, evidence)
		generations, genErr := r.loadGenerationsSince(ctx, fixture, since)
		if genErr == nil {
			r.reporter.ObserveGenerations(fixture.Key, generations, fixture.JudgeTokenJTI)
		} else {
			r.reporter.Trial(fixture.Key, "eval failed to load generation metadata", "error", genErr.Error())
		}
		if evidenceHasActivity(evidence) || len(generations) > 0 {
			seenActivity = true
		}
		if !seenActivity && time.Now().After(noActivityDeadline) {
			return last, fmt.Errorf("trial had no runtime activity after %s: gateway accepted but no session events, specialist tasks, or generations were observed", time.Since(since).Round(time.Second))
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
		signature := gradeCheckpointSignature(evidence, reason, decision)
		if shouldLogEvalCheckpoint(signature, lastCheckpointSignature, lastCheckpointAt) {
			lastCheckpointSignature = signature
			lastCheckpointAt = time.Now()
			r.reporter.Trial(fixture.Key, "eval grade checkpoint",
				"passed", passed,
				"reason", reason,
				"behavior", decision.Behavior,
				"specialist", decision.SpecialistSlug,
				"final_text_present", strings.TrimSpace(evidence.FinalText) != "",
				"tasks", len(evidence.Tasks),
				"tool_calls", len(evidence.ToolCalls),
				"events", len(evidence.Events),
				"generations", len(generations),
			)
		}
		if IsTerminal(c, evidence) && !needsMoreObservation(c, evidence) {
			return last, nil
		}
		time.Sleep(evalPollInterval)
	}
	return last, fmt.Errorf("trial timed out after %s: %s", timeout, last.Reason)
}

func evidenceHasActivity(evidence Evidence) bool {
	return len(evidence.Events) > 0 || len(evidence.Tasks) > 0
}

func shouldLogEvalCheckpoint(signature, lastSignature string, lastAt time.Time) bool {
	return signature != lastSignature || lastAt.IsZero() || time.Since(lastAt) >= evalCheckpointInterval
}

func gradeCheckpointSignature(evidence Evidence, reason string, decision Decision) string {
	return fmt.Sprintf("%s|%s|%s|%t|%d|%d|%d",
		reason,
		decision.Behavior,
		decision.SpecialistSlug,
		strings.TrimSpace(evidence.FinalText) != "",
		len(evidence.Tasks),
		len(evidence.ToolCalls),
		len(evidence.Events),
	)
}

func minDuration(left, right time.Duration) time.Duration {
	if left < right {
		return left
	}
	return right
}
