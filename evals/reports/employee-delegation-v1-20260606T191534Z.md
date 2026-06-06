# Employee Delegation Eval Report - 2026-06-06 19:15 UTC

Suite: `employee-delegation-v1`

Artifact source: `tmp/evals/runs/20260606T191534Z-bb746ff2`

Run command:

```sh
make employee-eval EVAL_MODELS=deepseek-v4-flash,qwen3.7-plus,grok-4.3 EVAL_RUNS=6 EVAL_PARALLEL=3
```

The run completed. The terminal output ended with `deps closed` at 2026-06-06 20:50:57 Africa/Lagos.

## Summary

Overall pass rate: 86.1% (217/252)

Overall metrics:

| Metric | Value |
| --- | ---: |
| Delegation accuracy | 81.2% |
| Correct specialist | 81.2% |
| False delegation | 3.7% |
| Clarify accuracy | 92.6% |
| Direct response accuracy | 94.4% |
| Average decision time | 15.0s |
| Average cost | $0.008399 |
| Average credits | 2.63 |

## Model Results

| Model | Pass rate | Passed | Delegation | Correct specialist | False delegation | Clarify | Direct | Avg decision | Avg cost | Avg credits |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `grok-4.3` | 88.1% | 74/84 | 83.3% | 83.3% | 5.6% | 88.9% | 100.0% | 6.6s | $0.015239 | 4.12 |
| `qwen3.7-plus` | 85.7% | 72/84 | 79.2% | 79.2% | 2.8% | 94.4% | 94.4% | 20.9s | $0.006581 | 2.38 |
| `deepseek-v4-flash` | 84.5% | 71/84 | 81.2% | 81.2% | 2.8% | 94.4% | 88.9% | 17.5s | $0.003377 | 1.38 |

## Failure Breakdown

Total failures: 35

| Failure category | Count |
| --- | ---: |
| Missed expected specialist delegation | 20 |
| Error | 10 |
| Launched specialist for non-delegation case | 4 |
| Specialist brief missing required content | 1 |

Failure categories by model:

| Model | Error | Missed delegation | False delegation | Brief issue |
| --- | ---: | ---: | ---: | ---: |
| `deepseek-v4-flash` | 3 | 8 | 1 | 1 |
| `qwen3.7-plus` | 2 | 9 | 1 | 0 |
| `grok-4.3` | 5 | 3 | 2 | 0 |

## Case Hotspots

| Case | Failed / total | Main issue |
| --- | ---: | --- |
| `clarify-then-dispatch-page` | 11/18 | Clarification follow-up judge instability plus missed delegation after clarification |
| `margin-calculator-build` | 7/18 | Models answered directly instead of delegating software engineering work |
| `memory-actionable-wedding-page` | 7/18 | Models failed to use preloaded memory as enough signal to delegate |
| `vague-bakery-site` | 4/18 | False delegation on a vague request that should clarify |
| `forget-bad-price-target` | 3/18 | Hindsight retain setup timed out before trial execution |
| `pricing-position-research` | 2/18 | Missed business research delegation |
| `competitor-check` | 1/18 | Missed business research delegation |

Cases with no failures:

- `corporate-catering`
- `flaky-site`
- `long-running-specialist-wake`
- `simple-intro`
- `too-ambiguous`
- `trivial-copy-edit`
- `wedding-page-detailed`

## Runtime And Infrastructure Errors

These 10 failures should not be interpreted as pure model decision failures.

| Error | Count |
| --- | ---: |
| `generate clarification follow-up: follow-up judge failed after 3 attempts: decode follow-up JSON: unexpected end of JSON input` | 6 |
| `retain eval memories: Post "http://localhost:8888/v1/default/banks/.../memories": context deadline exceeded` | 3 |
| `judge final response: judge status 502: map[error:upstream unreachable]` | 1 |

## Highest Cost Runs

| Model | Case | Run | Cost | Credits | Decision | Generations |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| `grok-4.3` | `long-running-specialist-wake` | 1 | $0.046977 | 41 | 8.9s | 8 |
| `grok-4.3` | `long-running-specialist-wake` | 5 | $0.045486 | 20 | 7.6s | 7 |
| `grok-4.3` | `long-running-specialist-wake` | 4 | $0.043193 | 34 | 8.7s | 9 |
| `grok-4.3` | `long-running-specialist-wake` | 3 | $0.042806 | 30 | 7.1s | 8 |
| `grok-4.3` | `long-running-specialist-wake` | 2 | $0.041933 | 24 | 9.9s | 9 |
| `grok-4.3` | `long-running-specialist-wake` | 6 | $0.040096 | 21 | 8.9s | 7 |
| `grok-4.3` | `memory-actionable-wedding-page` | 1 | $0.040043 | 0 | 25.8s | 3 |
| `grok-4.3` | `memory-actionable-wedding-page` | 2 | $0.037006 | 10 | 22.6s | 3 |

## Slowest Decision Runs

| Model | Case | Run | Decision | Cost | Generations |
| --- | --- | ---: | ---: | ---: | ---: |
| `qwen3.7-plus` | `memory-actionable-wedding-page` | 4 | 71.3s | $0.013805 | 4 |
| `qwen3.7-plus` | `margin-calculator-build` | 5 | 70.5s | $0.009930 | 2 |
| `deepseek-v4-flash` | `memory-actionable-wedding-page` | 6 | 65.6s | $0.005738 | 6 |
| `qwen3.7-plus` | `margin-calculator-build` | 2 | 65.1s | $0.009635 | 2 |
| `qwen3.7-plus` | `margin-calculator-build` | 3 | 59.9s | $0.009190 | 2 |
| `qwen3.7-plus` | `margin-calculator-build` | 4 | 59.1s | $0.009066 | 2 |
| `deepseek-v4-flash` | `memory-actionable-wedding-page` | 5 | 55.1s | $0.004336 | 6 |
| `qwen3.7-plus` | `memory-actionable-wedding-page` | 6 | 54.1s | $0.010588 | 3 |

## Interpretation

- `grok-4.3` had the best pass rate and fastest average decision time, but it was the most expensive and had the highest false delegation rate.
- `deepseek-v4-flash` remained the cheapest and was close to the other models on pass rate, but struggled with some delegation-after-context cases.
- `qwen3.7-plus` was strong on clarification/direct-answer accuracy, but slowest overall and weak on `margin-calculator-build`.
- The cleanest product signal is that long-running delegation now passed across all 18 runs.
- The biggest model-behavior gap is still deciding that memory/context is enough signal to delegate without asking again or answering directly.
- The biggest eval-infrastructure gap is judge output reliability for generated clarification follow-ups.

## Full Failure List

### deepseek-v4-flash / vague-bakery-site / run 5

- Reason: launched specialist for non-delegation case
- Expected: clarify ``
- Actual: delegate `software-engineering-specialist`

### deepseek-v4-flash / competitor-check / run 5

- Reason: missed expected specialist delegation
- Expected: delegate `business-research-specialist`
- Actual: clarify ``

### deepseek-v4-flash / clarify-then-dispatch-page / run 2

- Reason: missed expected specialist delegation
- Expected: delegate `software-engineering-specialist`
- Actual: direct ``

### deepseek-v4-flash / clarify-then-dispatch-page / run 3

- Reason: error
- Error: generate clarification follow-up: follow-up judge failed after 3 attempts: decode follow-up JSON: unexpected end of JSON input
- Expected: delegate `software-engineering-specialist`
- Actual: clarify ``

### deepseek-v4-flash / clarify-then-dispatch-page / run 4

- Reason: missed expected specialist delegation
- Expected: delegate `software-engineering-specialist`
- Actual: direct ``

### deepseek-v4-flash / clarify-then-dispatch-page / run 5

- Reason: missed expected specialist delegation
- Expected: delegate `software-engineering-specialist`
- Actual: clarify ``

### deepseek-v4-flash / clarify-then-dispatch-page / run 6

- Reason: specialist brief missing "corporate catering"
- Expected: delegate `software-engineering-specialist`
- Actual: delegate `software-engineering-specialist`

### deepseek-v4-flash / memory-actionable-wedding-page / run 5

- Reason: missed expected specialist delegation
- Expected: delegate `software-engineering-specialist`
- Actual: direct ``

### deepseek-v4-flash / memory-actionable-wedding-page / run 6

- Reason: missed expected specialist delegation
- Expected: delegate `software-engineering-specialist`
- Actual: clarify ``

### deepseek-v4-flash / forget-bad-price-target / run 5

- Reason: error
- Error: retain eval memories: Post "http://localhost:8888/v1/default/banks/org-0dcb0d24-efbb-452d-a56f-2b4f0d4df574/memories": context deadline exceeded
- Expected: direct ``
- Actual:  ``

### deepseek-v4-flash / forget-bad-price-target / run 6

- Reason: error
- Error: retain eval memories: Post "http://localhost:8888/v1/default/banks/org-109ec6a6-e32a-4ed4-b7a0-38150be4d3d0/memories": context deadline exceeded
- Expected: direct ``
- Actual:  ``

### deepseek-v4-flash / pricing-position-research / run 3

- Reason: missed expected specialist delegation
- Expected: delegate `business-research-specialist`
- Actual: clarify ``

### deepseek-v4-flash / margin-calculator-build / run 2

- Reason: missed expected specialist delegation
- Expected: delegate `software-engineering-specialist`
- Actual: clarify ``

### qwen3.7-plus / vague-bakery-site / run 4

- Reason: launched specialist for non-delegation case
- Expected: clarify ``
- Actual: delegate `software-engineering-specialist`

### qwen3.7-plus / clarify-then-dispatch-page / run 4

- Reason: error
- Error: generate clarification follow-up: follow-up judge failed after 3 attempts: decode follow-up JSON: unexpected end of JSON input
- Expected: delegate `software-engineering-specialist`
- Actual: clarify ``

### qwen3.7-plus / memory-actionable-wedding-page / run 4

- Reason: missed expected specialist delegation
- Expected: delegate `software-engineering-specialist`
- Actual: direct ``

### qwen3.7-plus / memory-actionable-wedding-page / run 5

- Reason: missed expected specialist delegation
- Expected: delegate `software-engineering-specialist`
- Actual: direct ``

### qwen3.7-plus / memory-actionable-wedding-page / run 6

- Reason: missed expected specialist delegation
- Expected: delegate `software-engineering-specialist`
- Actual: direct ``

### qwen3.7-plus / forget-bad-price-target / run 1

- Reason: error
- Error: retain eval memories: Post "http://localhost:8888/v1/default/banks/org-f1059d67-4488-4161-b8e5-5d14ea02fe0a/memories": context deadline exceeded
- Expected: direct ``
- Actual:  ``

### qwen3.7-plus / pricing-position-research / run 2

- Reason: missed expected specialist delegation
- Expected: delegate `business-research-specialist`
- Actual: direct ``

### qwen3.7-plus / margin-calculator-build / run 2

- Reason: missed expected specialist delegation
- Expected: delegate `software-engineering-specialist`
- Actual: direct ``

### qwen3.7-plus / margin-calculator-build / run 3

- Reason: missed expected specialist delegation
- Expected: delegate `software-engineering-specialist`
- Actual: direct ``

### qwen3.7-plus / margin-calculator-build / run 4

- Reason: missed expected specialist delegation
- Expected: delegate `software-engineering-specialist`
- Actual: direct ``

### qwen3.7-plus / margin-calculator-build / run 5

- Reason: missed expected specialist delegation
- Expected: delegate `software-engineering-specialist`
- Actual: direct ``

### qwen3.7-plus / margin-calculator-build / run 6

- Reason: missed expected specialist delegation
- Expected: delegate `software-engineering-specialist`
- Actual: direct ``

### grok-4.3 / vague-bakery-site / run 2

- Reason: launched specialist for non-delegation case
- Expected: clarify ``
- Actual: delegate `software-engineering-specialist`

### grok-4.3 / vague-bakery-site / run 3

- Reason: launched specialist for non-delegation case
- Expected: clarify ``
- Actual: delegate `software-engineering-specialist`

### grok-4.3 / clarify-then-dispatch-page / run 1

- Reason: error
- Error: generate clarification follow-up: follow-up judge failed after 3 attempts: decode follow-up JSON: unexpected end of JSON input
- Expected: delegate `software-engineering-specialist`
- Actual: clarify ``

### grok-4.3 / clarify-then-dispatch-page / run 3

- Reason: error
- Error: generate clarification follow-up: follow-up judge failed after 3 attempts: decode follow-up JSON: unexpected end of JSON input
- Expected: delegate `software-engineering-specialist`
- Actual: clarify ``

### grok-4.3 / clarify-then-dispatch-page / run 4

- Reason: error
- Error: generate clarification follow-up: follow-up judge failed after 3 attempts: decode follow-up JSON: unexpected end of JSON input
- Expected: delegate `software-engineering-specialist`
- Actual: clarify ``

### grok-4.3 / clarify-then-dispatch-page / run 5

- Reason: error
- Error: generate clarification follow-up: follow-up judge failed after 3 attempts: decode follow-up JSON: unexpected end of JSON input
- Expected: delegate `software-engineering-specialist`
- Actual: clarify ``

### grok-4.3 / clarify-then-dispatch-page / run 6

- Reason: missed expected specialist delegation
- Expected: delegate `software-engineering-specialist`
- Actual: direct ``

### grok-4.3 / memory-actionable-wedding-page / run 1

- Reason: missed expected specialist delegation
- Expected: delegate `software-engineering-specialist`
- Actual: direct ``

### grok-4.3 / memory-actionable-wedding-page / run 2

- Reason: missed expected specialist delegation
- Expected: delegate `software-engineering-specialist`
- Actual: direct ``

### grok-4.3 / margin-calculator-build / run 5

- Reason: error
- Error: judge final response: judge status 502: map[error:upstream unreachable]
- Expected: delegate `software-engineering-specialist`
- Actual: pending ``
