package correlation

import (
	"context"
	"net/http"
	"strings"

	sentrygo "github.com/getsentry/sentry-go"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/logging"
)

const (
	HeaderSessionID             = "X-Hivy-Session-ID"
	HeaderProvisioningAttemptID = "X-Hivy-Provisioning-Attempt-ID"
	HeaderTraceID               = "X-Hivy-Trace-ID"
	HeaderSandboxID             = "X-Hivy-Sandbox-ID"

	LabelSessionID             = "session_id"
	LabelProvisioningAttemptID = "provisioning_attempt_id"
	LabelTraceID               = "trace_id"
	LabelSandboxID             = "sandbox_id"
	LabelOrgID                 = "org_id"
	LabelAgentID               = "agent_id"
)

type contextKey struct{}

// Values are the stable identifiers used to join one session across the API,
// task worker, sandbox control plane, runner, and runtime.
type Values struct {
	SessionID             string
	ProvisioningAttemptID string
	TraceID               string
	SandboxID             string
	OrgID                 string
	AgentID               string
}

func NewProvisioning(sessionID uuid.UUID) Values {
	attemptID := uuid.New()
	return Values{
		SessionID:             idString(sessionID),
		ProvisioningAttemptID: attemptID.String(),
		TraceID:               strings.ReplaceAll(attemptID.String(), "-", ""),
	}
}

func FromContext(ctx context.Context) Values {
	if ctx == nil {
		return Values{}
	}
	values, _ := ctx.Value(contextKey{}).(Values)
	return values
}

// WithValues merges non-empty values into ctx and appends them to the
// contextual logger. Call it once at each service boundary.
func WithValues(ctx context.Context, incoming Values) context.Context {
	current := FromContext(ctx)
	previous := current
	current.merge(incoming)
	ctx = context.WithValue(ctx, contextKey{}, current)
	applySentry(ctx, current)
	return logging.WithAttrs(ctx, current.changedLogAttrs(previous)...)
}

func WithSandboxID(ctx context.Context, sandboxID string) context.Context {
	return WithValues(ctx, Values{SandboxID: sandboxID})
}

func FromHeaders(header http.Header) Values {
	if header == nil {
		return Values{}
	}
	return Values{
		SessionID:             safeValue(header.Get(HeaderSessionID)),
		ProvisioningAttemptID: safeValue(header.Get(HeaderProvisioningAttemptID)),
		TraceID:               safeValue(header.Get(HeaderTraceID)),
		SandboxID:             safeValue(header.Get(HeaderSandboxID)),
	}
}

func InjectHeaders(ctx context.Context, header http.Header) {
	if header == nil {
		return
	}
	values := FromContext(ctx)
	setHeader(header, HeaderSessionID, values.SessionID)
	setHeader(header, HeaderProvisioningAttemptID, values.ProvisioningAttemptID)
	setHeader(header, HeaderTraceID, values.TraceID)
	setHeader(header, HeaderSandboxID, values.SandboxID)
}

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := WithValues(r.Context(), FromHeaders(r.Header))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func FromLabels(labels map[string]string) Values {
	return Values{
		SessionID:             safeValue(labels[LabelSessionID]),
		ProvisioningAttemptID: safeValue(labels[LabelProvisioningAttemptID]),
		TraceID:               safeValue(labels[LabelTraceID]),
		SandboxID:             safeValue(labels[LabelSandboxID]),
		OrgID:                 safeValue(labels[LabelOrgID]),
		AgentID:               safeValue(labels[LabelAgentID]),
	}
}

func Merge(base, incoming Values) Values {
	base.merge(incoming)
	return base
}

func ApplyLabels(labels map[string]string, values Values) {
	if labels == nil {
		return
	}
	setLabel(labels, LabelSessionID, values.SessionID)
	setLabel(labels, LabelProvisioningAttemptID, values.ProvisioningAttemptID)
	setLabel(labels, LabelTraceID, values.TraceID)
	setLabel(labels, LabelSandboxID, values.SandboxID)
	setLabel(labels, LabelOrgID, values.OrgID)
	setLabel(labels, LabelAgentID, values.AgentID)
}

func (v *Values) merge(incoming Values) {
	if value := safeValue(incoming.SessionID); value != "" {
		v.SessionID = value
	}
	if value := safeValue(incoming.ProvisioningAttemptID); value != "" {
		v.ProvisioningAttemptID = value
	}
	if value := safeValue(incoming.TraceID); value != "" {
		v.TraceID = value
	}
	if value := safeValue(incoming.SandboxID); value != "" {
		v.SandboxID = value
	}
	if value := safeValue(incoming.OrgID); value != "" {
		v.OrgID = value
	}
	if value := safeValue(incoming.AgentID); value != "" {
		v.AgentID = value
	}
}

func (v Values) changedLogAttrs(previous Values) []any {
	attrs := make([]any, 0, 12)
	appendAttr := func(key, value, old string) {
		if value != "" && value != old {
			attrs = append(attrs, key, value)
		}
	}
	appendAttr(LabelSessionID, v.SessionID, previous.SessionID)
	appendAttr(LabelProvisioningAttemptID, v.ProvisioningAttemptID, previous.ProvisioningAttemptID)
	appendAttr(LabelTraceID, v.TraceID, previous.TraceID)
	appendAttr(LabelSandboxID, v.SandboxID, previous.SandboxID)
	appendAttr(LabelOrgID, v.OrgID, previous.OrgID)
	appendAttr(LabelAgentID, v.AgentID, previous.AgentID)
	return attrs
}

func setHeader(header http.Header, key, value string) {
	if value = safeValue(value); value != "" {
		header.Set(key, value)
	}
}

func setLabel(labels map[string]string, key, value string) {
	if value = safeValue(value); value != "" {
		labels[key] = value
	}
}

func safeValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 128 {
		return value[:128]
	}
	return value
}

func idString(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	return id.String()
}

func applySentry(ctx context.Context, values Values) {
	if hub := sentrygo.GetHubFromContext(ctx); hub != nil {
		scope := hub.Scope()
		if values.SessionID != "" {
			scope.SetTag(LabelSessionID, values.SessionID)
		}
		if values.ProvisioningAttemptID != "" {
			scope.SetTag(LabelProvisioningAttemptID, values.ProvisioningAttemptID)
		}
		if values.TraceID != "" {
			scope.SetTag("provisioning_trace_id", values.TraceID)
		}
		if values.SandboxID != "" {
			scope.SetTag(LabelSandboxID, values.SandboxID)
		}
	}
	if span := sentrygo.SpanFromContext(ctx); span != nil {
		if values.SessionID != "" {
			span.SetData(LabelSessionID, values.SessionID)
		}
		if values.ProvisioningAttemptID != "" {
			span.SetData(LabelProvisioningAttemptID, values.ProvisioningAttemptID)
		}
		if values.TraceID != "" {
			span.SetData("provisioning_trace_id", values.TraceID)
		}
		if values.SandboxID != "" {
			span.SetData(LabelSandboxID, values.SandboxID)
		}
	}
}
