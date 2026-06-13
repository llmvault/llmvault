package e2e

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

type agentRuntimeE2ETrace struct {
	t          *testing.T
	startedAt  time.Time
	fullBodies bool
	mu         sync.Mutex
	seq        int
}

func requireAgentRuntimeE2EVerbose(t *testing.T) {
	t.Helper()
	if !testing.Verbose() {
		t.Fatalf("flagship runtime E2E requires live trace output; rerun with go test -v")
	}
}

func newAgentRuntimeE2ETrace(t *testing.T) *agentRuntimeE2ETrace {
	t.Helper()
	compact := os.Getenv("HIVY_AGENT_RUNTIME_E2E_TRACE_COMPACT") == "1"
	return &agentRuntimeE2ETrace{
		t:          t,
		startedAt:  time.Now(),
		fullBodies: os.Getenv("HIVY_AGENT_RUNTIME_E2E_TRACE_FULL") == "1" || !compact,
	}
}

func (tr *agentRuntimeE2ETrace) Logf(component, format string, args ...any) {
	if tr == nil {
		return
	}
	tr.mu.Lock()
	tr.seq++
	seq := tr.seq
	elapsed := time.Since(tr.startedAt).Truncate(time.Millisecond)
	tr.mu.Unlock()
	tr.t.Helper()
	tr.t.Logf("[agent-runtime-e2e %04d +%s] %-16s %s", seq, elapsed, component, redactE2ETrace(fmt.Sprintf(format, args...)))
}

func (tr *agentRuntimeE2ETrace) Body(component, label string, body []byte) {
	if tr == nil {
		return
	}
	sum := sha256.Sum256(body)
	content := string(body)
	if !tr.fullBodies && len(content) > 6000 {
		content = content[:6000] + fmt.Sprintf("... [truncated %d bytes; unset HIVY_AGENT_RUNTIME_E2E_TRACE_COMPACT for full body]", len(body)-6000)
	}
	tr.Logf(component, "%s bytes=%d sha256=%s body=%s", label, len(body), hex.EncodeToString(sum[:]), content)
}

var (
	e2eTraceBearerRedactor     = regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9._~+/=-]+`)
	e2eTraceOpenRouterRedactor = regexp.MustCompile(`sk-or-v1-[A-Za-z0-9._-]+`)
)

func redactE2ETrace(value string) string {
	value = strings.ReplaceAll(value, agentRuntimeProxyToken, "ptok_[redacted]")
	value = strings.ReplaceAll(value, "agent-runtime-e2e-secret", "runtime-secret-[redacted]")
	value = e2eTraceBearerRedactor.ReplaceAllString(value, "Bearer [redacted]")
	value = e2eTraceOpenRouterRedactor.ReplaceAllString(value, "sk-or-v1-[redacted]")
	return value
}
