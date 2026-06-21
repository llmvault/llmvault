package microsandbox

import (
	"net/http"
	"testing"
)

func jsonNumberArray(t *testing.T, value any) []int {
	t.Helper()
	raw, ok := value.([]any)
	if !ok {
		t.Fatalf("value = %T, want array", value)
	}
	out := make([]int, 0, len(raw))
	for _, item := range raw {
		number, ok := item.(float64)
		if !ok {
			t.Fatalf("array item = %T, want number", item)
		}
		out = append(out, int(number))
	}
	return out
}

func assertRuntimeHealthCheck(t *testing.T, createReq map[string]any, port int) {
	t.Helper()
	checks, ok := createReq["health_checks"].([]any)
	if !ok || len(checks) != 1 {
		t.Fatalf("health_checks = %#v, want one check", createReq["health_checks"])
	}
	check, ok := checks[0].(map[string]any)
	if !ok {
		t.Fatalf("health check = %T, want object", checks[0])
	}
	if check["guest_port"] != float64(port) ||
		check["type"] != "http" ||
		check["method"] != "GET" ||
		check["path"] != "/healthz" ||
		check["expected_status"] != float64(http.StatusOK) ||
		check["timeout_seconds"] != float64(30) ||
		check["interval_ms"] != float64(250) {
		t.Fatalf("health check = %#v", check)
	}
}
