package tasks

// Every task constructor must return options separately (not baked into the
// asynq.Task) so they survive the enqueue client's Sentry trace-payload rewrite,
// and every periodic config must carry asynq.Unique against duplicate ticks.

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/usehivy/hivy/internal/model"
)

func uniqueTTLFromOptions(opts []asynq.Option) (time.Duration, bool) {
	for _, opt := range opts {
		if opt.Type() == asynq.UniqueOpt {
			if ttl, ok := opt.Value().(time.Duration); ok {
				return ttl, true
			}
		}
	}
	return 0, false
}

// Constructors must return the queue routing option in a separate slice: a
// baked-in option is silently stripped when the enqueue client rebuilds the task
// to wrap the Sentry trace header into the payload.
func TestAllConvergedConstructorsReturnOptionsSeparately(t *testing.T) {
	type constructorTest struct {
		name          string
		expectedQueue string
		run           func() ([]asynq.Option, error)
	}

	tests := []constructorTest{
		{
			name:          "EmailSend",
			expectedQueue: QueueDefault,
			run: func() ([]asynq.Option, error) {
				_, opts, err := NewEmailSendTask("a@b.com", "subject", "body")
				return opts, err
			},
		},
		{
			name:          "EmailSendTemplate",
			expectedQueue: QueueDefault,
			run: func() ([]asynq.Option, error) {
				_, opts, err := NewEmailSendTemplateTask("a@b.com", "welcome", nil)
				return opts, err
			},
		},
		{
			name:          "APIKeyUpdate",
			expectedQueue: QueueBulk,
			run: func() ([]asynq.Option, error) {
				_, opts, err := NewAPIKeyUpdateTask(uuid.New())
				return opts, err
			},
		},
		{
			name:          "AuditWrite",
			expectedQueue: QueueBulk,
			run: func() ([]asynq.Option, error) {
				_, opts, err := NewAuditWriteTask(model.AuditEntry{Action: "test"})
				return opts, err
			},
		},
		{
			name:          "GenerationWrite",
			expectedQueue: QueueBulk,
			run: func() ([]asynq.Option, error) {
				_, opts, err := NewGenerationWriteTask(model.Generation{})
				return opts, err
			},
		},
		{
			name:          "EmployeeCleanup",
			expectedQueue: QueueDefault,
			run: func() ([]asynq.Option, error) {
				_, opts, err := NewEmployeeCleanupTask(uuid.New())
				return opts, err
			},
		},
		{
			name:          "SandboxTemplateBuild",
			expectedQueue: QueueDefault,
			run: func() ([]asynq.Option, error) {
				_, opts, err := NewSandboxTemplateBuildTask(uuid.New())
				return opts, err
			},
		},
		{
			name:          "SandboxTemplateRetryBuild",
			expectedQueue: QueueDefault,
			run: func() ([]asynq.Option, error) {
				_, opts, err := NewSandboxTemplateRetryBuildTask(uuid.New(), nil)
				return opts, err
			},
		},
		{
			name:          "SkillHydrate",
			expectedQueue: QueueDefault,
			run: func() ([]asynq.Option, error) {
				_, opts, err := NewSkillHydrateTask(uuid.New())
				return opts, err
			},
		},
		{
			name:          "EmployeeMemoryRetain",
			expectedQueue: QueueDefault,
			run: func() ([]asynq.Option, error) {
				_, opts, err := NewEmployeeMemoryRetainTask(EmployeeMemoryRetainPayload{
					EmployeeID: uuid.New(),
					SandboxID:  uuid.New(),
					SessionID:  "sess-1",
				})
				return opts, err
			},
		},
		{
			name:          "EmployeeMemoryRefresh",
			expectedQueue: QueueDefault,
			run: func() ([]asynq.Option, error) {
				_, opts, err := NewEmployeeMemoryRefreshTask(EmployeeMemoryRefreshPayload{
					EmployeeID: uuid.New(),
				})
				return opts, err
			},
		},
		{
			name:          "GatewaySlack",
			expectedQueue: QueueCritical,
			run: func() ([]asynq.Option, error) {
				_, opts, err := NewGatewaySlackTask(GatewaySlackPayload{
					ConnectionID: "c1",
					OrgID:        "o1",
					EmployeeID:   "e1",
				})
				return opts, err
			},
		},
		{
			name:          "GatewayExternalCallback",
			expectedQueue: QueueCritical,
			run: func() ([]asynq.Option, error) {
				_, opts, err := NewGatewayExternalCallbackTask(GatewayExternalCallbackPayload{
					RouteID:    "r1",
					OrgID:      "o1",
					EmployeeID: "e1",
				})
				return opts, err
			},
		},
		{
			name:          "GatewaySlackStatus",
			expectedQueue: QueueCritical,
			run: func() ([]asynq.Option, error) {
				_, opts, err := NewGatewaySlackStatusTask(GatewaySlackStatusPayload{
					ConnectionID: "c1",
					OrgID:        "o1",
					EmployeeID:   "e1",
				})
				return opts, err
			},
		},
		{
			name:          "ConversationName",
			expectedQueue: QueueBulk,
			run: func() ([]asynq.Option, error) {
				_, opts, err := NewConversationNameTask(uuid.New())
				return opts, err
			},
		},
		{
			name:          "WebhookForward",
			expectedQueue: QueueCritical,
			run: func() ([]asynq.Option, error) {
				_, opts, err := NewWebhookForwardTask("http://example.com", nil, []byte("{}"))
				return opts, err
			},
		},
		{
			name:          "EmployeeTriggerDispatch",
			expectedQueue: QueueCritical,
			run: func() ([]asynq.Option, error) {
				_, opts, err := NewEmployeeTriggerDispatchTask(EmployeeTriggerDispatchPayload{
					OrgID:      uuid.New(),
					DeliveryID: "d1",
				})
				return opts, err
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts, err := tc.run()
			if err != nil {
				t.Fatalf("constructor error: %v", err)
			}
			if len(opts) == 0 {
				t.Fatal("options slice is empty — options must be returned separately (P0-11)")
			}
			queue, ok := queueFromOptions(opts)
			if !ok {
				t.Fatalf("no Queue option found in returned options slice (P0-11)")
			}
			if queue != tc.expectedQueue {
				t.Fatalf("queue = %q, want %q", queue, tc.expectedQueue)
			}
		})
	}
}

// Every periodic config must carry asynq.Unique so N worker-replica schedulers
// firing at the same tick don't enqueue N copies of the same sweep.
func TestPeriodicConfigsCarryDedupeOptions(t *testing.T) {
	configs := PeriodicTaskConfigs(nil, nil)
	if len(configs) == 0 {
		t.Fatal("PeriodicTaskConfigs returned empty slice")
	}
	for _, cfg := range configs {
		t.Run(cfg.Task.Type(), func(t *testing.T) {
			ttl, ok := uniqueTTLFromOptions(cfg.Opts)
			if !ok {
				t.Errorf("periodic config for %q is missing asynq.Unique(…) — N scheduler replicas will fire N duplicate ticks (P1-23)", cfg.Task.Type())
				return
			}
			if ttl <= 0 {
				t.Errorf("periodic config for %q has Unique(0) — must be positive (P1-23)", cfg.Task.Type())
			}
		})
	}
}
