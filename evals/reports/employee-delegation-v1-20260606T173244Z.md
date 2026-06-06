# Employee Delegation Eval Report - 2026-06-06 17:32 UTC

Suite: `employee-delegation-v1`

Artifact source: `tmp/evals/runs/20260606T173244Z-67da76ea`

## Summary

Overall pass rate: 78.3% (329/420)

Overall metrics:

| Metric | Value |
| --- | ---: |
| Delegation accuracy | 75.8% |
| Correct specialist | 75.8% |
| False delegation | 6.1% |
| Clarify accuracy | 80.0% |
| Direct response accuracy | 84.4% |
| Average decision time | 14.9s |
| Average cost | $0.008127 |
| Average credits | 3.01 |

## Model Results

| Model | Pass rate | Passed | Delegation | Correct specialist | False delegation | Avg decision | Avg cost | Avg credits |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `qwen3.7-plus` | 94.0% | 79/84 | 89.6% | 89.6% | 0.0% | 22.6s | $0.007378 | 1.51 |
| `grok-4.3` | 85.7% | 72/84 | 83.3% | 83.3% | 11.1% | 8.3s | $0.016185 | 4.06 |
| `deepseek-v4-flash` | 84.5% | 71/84 | 77.1% | 77.1% | 5.6% | 19.3s | $0.002869 | 1.39 |
| `nemotron-3-ultra-550b-a55b` | 66.7% | 56/84 | 66.7% | 66.7% | 8.3% | 19.9s | $0.011191 | 6.58 |
| `gemini-3.1-flash-lite` | 60.7% | 51/84 | 62.5% | 62.5% | 5.6% | 4.3s | $0.003012 | 1.52 |

## Failure Breakdown

Total failures: 91

| Failure category | Count |
| --- | ---: |
| Error | 55 |
| Missed expected specialist delegation | 24 |
| Launched specialist for non-delegation case | 11 |
| Specialist brief missing required content | 1 |

Failure categories by model:

| Model | Error | Missed delegation | False delegation | Brief issue |
| --- | ---: | ---: | ---: | ---: |
| `deepseek-v4-flash` | 2 | 9 | 2 | 0 |
| `qwen3.7-plus` | 0 | 5 | 0 | 0 |
| `grok-4.3` | 2 | 6 | 4 | 0 |
| `nemotron-3-ultra-550b-a55b` | 23 | 2 | 3 | 0 |
| `gemini-3.1-flash-lite` | 28 | 2 | 2 | 1 |

## Case Hotspots

| Case | Failed / total |
| --- | ---: |
| `memory-actionable-wedding-page` | 18/30 |
| `margin-calculator-build` | 14/30 |
| `vague-bakery-site` | 14/30 |
| `clarify-then-dispatch-page` | 12/30 |
| `forget-bad-price-target` | 12/30 |
| `corporate-catering` | 6/30 |
| `long-running-specialist-wake` | 4/30 |
| `flaky-site` | 3/30 |
| `pricing-position-research` | 3/30 |
| `simple-intro` | 2/30 |
| `competitor-check` | 1/30 |
| `too-ambiguous` | 1/30 |
| `wedding-page-detailed` | 1/30 |
| `trivial-copy-edit` | 0/30 |

## Runtime And Infrastructure Errors

The reported pass rate includes infrastructure/runtime failures. These should not be interpreted as pure model decision failures.

| Error | Count |
| --- | ---: |
| `retain eval memories: hindsight retain: status 500: {"detail":"out of shared memory\nHINT:  You might need to increase \"max_locks_per_transaction\"."}` | 29 |
| `trial timed out after 10m0s: no delegation or final response observed` | 6 |
| `create eval user: ERROR: out of shared memory (SQLSTATE 53200)` | 3 |
| `generate clarification follow-up: follow-up judge failed after 3 attempts: decode follow-up JSON: unexpected end of JSON input` | 3 |
| `grant eval credits: ERROR: out of shared memory (SQLSTATE 53200)` | 3 |
| `ensure employee sandbox: updating sandbox: ERROR: out of shared memory (SQLSTATE 53200)` | 2 |
| `sync employee sandbox: mint proxy token: persist proxy token: ERROR: out of shared memory (SQLSTATE 53200)` | 2 |
| `ERROR: out of shared memory (SQLSTATE 53200)` | 1 |
| `create eval employee: ERROR: out of shared memory (SQLSTATE 53200)` | 1 |
| `create eval gateway route: ERROR: out of shared memory (SQLSTATE 53200)` | 1 |
| `ensure employee sandbox: persist proxy token: ERROR: out of shared memory (SQLSTATE 53200)` | 1 |
| `ensure employee sandbox: tag employee proxy token sandbox: ERROR: out of shared memory (SQLSTATE 53200)` | 1 |
| `gateway returned status 400: {"error":"find or create gateway session: ERROR: out of shared memory (SQLSTATE 53200)"}` | 1 |
| `load eval session events: context deadline exceeded` | 1 |

## Notes

- `qwen3.7-plus` had the strongest score in this run and zero false delegations, but it was also the slowest model on average.
- `deepseek-v4-flash` remained competitive on pass rate while being the cheapest model in this run.
- `grok-4.3` was fast and high scoring, but had the highest false delegation rate among the top three models.
- `gemini-3.1-flash-lite` and `nemotron-3-ultra-550b-a55b` were heavily affected by infrastructure errors, especially Postgres shared-memory exhaustion and Hindsight retain failures.
- The timeout failure `no delegation or final response observed` appeared 6 times and should be investigated separately from routing quality.
