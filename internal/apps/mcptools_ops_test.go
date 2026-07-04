package apps

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// TestAppStatusOutputMarshals is the regression guard for the prod bug where
// app_status returned "failed to serialize response" every call. The health
// field is appd's raw body and the json marshaler re-validates a RawMessage,
// so a non-empty-but-invalid probe body used to make json.Marshal fail. Each
// case must marshal successfully and carry the expected fields.
func TestAppStatusOutputMarshals(t *testing.T) {
	activeVersionID := uuid.New()
	baseApp := func(status string) *model.App {
		return &model.App{
			ID:     uuid.New(),
			Name:   "Flow App",
			Slug:   "flow-app",
			Status: status,
		}
	}

	cases := []struct {
		name       string
		status     *AppStatus
		url        string
		wantFields []string // keys expected present in the marshaled output
		missing    []string // keys expected absent
	}{
		{
			name:       "not deployed",
			status:     &AppStatus{App: baseApp(model.AppStatusDraft)},
			url:        "",
			wantFields: []string{"status", "name", "slug"},
			missing:    []string{"url", "active_version", "health", "health_error"},
		},
		{
			name: "running with valid health",
			status: &AppStatus{
				App: baseApp(model.AppStatusRunning),
				ActiveVersion: &model.AppVersion{
					ID:           activeVersionID,
					Notes:        "first release",
					BundleSHA256: "abc123",
					CreatedAt:    time.Now(),
				},
				// appd writes JSON via json.Encoder, i.e. with a trailing newline.
				Health: json.RawMessage(`{"ok":true,"app":{"running":true}}` + "\n"),
			},
			url:        "http://127.0.0.1:45678",
			wantFields: []string{"status", "name", "slug", "url", "active_version", "health"},
			missing:    []string{"health_error"},
		},
		{
			name: "health error (probe failed)",
			status: &AppStatus{
				App:         baseApp(model.AppStatusRunning),
				HealthError: "appd: GET /health: connection refused",
			},
			url:        "http://127.0.0.1:45678",
			wantFields: []string{"status", "name", "slug", "url", "health_error"},
			missing:    []string{"health"},
		},
		{
			// The exact prod trigger: a non-empty but invalid health body. Before
			// the fix this made json.Marshal fail; now it degrades to health_error.
			name: "invalid non-json health body",
			status: &AppStatus{
				App:    baseApp(model.AppStatusRunning),
				Health: json.RawMessage("   \n"),
			},
			url:        "http://127.0.0.1:45678",
			wantFields: []string{"status", "name", "slug", "url", "health_error"},
			missing:    []string{"health"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := appStatusOutput(tc.status, tc.url)
			b, err := json.Marshal(out)
			if err != nil {
				t.Fatalf("json.Marshal(app_status output) failed: %v", err)
			}
			var round map[string]any
			if err := json.Unmarshal(b, &round); err != nil {
				t.Fatalf("output is not valid JSON: %v", err)
			}
			for _, k := range tc.wantFields {
				if _, ok := round[k]; !ok {
					t.Fatalf("missing field %q in %s", k, string(b))
				}
			}
			for _, k := range tc.missing {
				if _, ok := round[k]; ok {
					t.Fatalf("unexpected field %q in %s", k, string(b))
				}
			}
		})
	}
}
