package runner

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
)

const sandboxLogIngestEnv = "HIVY_LOG_INGEST_URL"

var protectedLogFields = []string{
	"authorization",
	"Authorization",
	"cookie",
	"Cookie",
	"password",
	"token",
	"access_token",
	"refresh_token",
}

func (s *Server) LogRoutes() http.Handler {
	r := chi.NewRouter()
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok\n")
	})
	r.Get("/metrics", s.logIngestMetrics)
	r.Post("/v1/sandbox-logs/{sandboxID}/{token}/journald", s.ingestSandboxJournal)
	r.Post("/v1/sandbox-logs/{sandboxID}/{token}/journald/upload", s.ingestSandboxJournal)
	return r
}

func (s *Server) addSandboxLogIngestEnv(req *CreateSandboxRequest) error {
	if req == nil || strings.TrimSpace(req.ID) == "" {
		return fmt.Errorf("sandbox id is required")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(s.cfg.RunnerLogIngestPublicURL), "/")
	if baseURL == "" || strings.TrimSpace(s.cfg.RunnerLogIngestSigningKey) == "" {
		return fmt.Errorf("runner log ingestion is not configured")
	}
	if req.Env == nil {
		req.Env = map[string]string{}
	}
	token := s.sandboxLogToken(req.ID)
	req.Env[sandboxLogIngestEnv] = fmt.Sprintf(
		"%s/v1/sandbox-logs/%s/%s/journald",
		baseURL,
		url.PathEscape(req.ID),
		url.PathEscape(token),
	)
	return nil
}

func (s *Server) sandboxLogToken(sandboxID string) string {
	mac := hmac.New(sha256.New, []byte(s.cfg.RunnerLogIngestSigningKey))
	_, _ = io.WriteString(mac, s.cfg.RunnerName)
	_, _ = mac.Write([]byte{0})
	_, _ = io.WriteString(mac, sandboxID)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Server) validSandboxLogToken(sandboxID, token string) bool {
	expected := s.sandboxLogToken(sandboxID)
	return hmac.Equal([]byte(expected), []byte(token))
}

func (s *Server) ingestSandboxJournal(w http.ResponseWriter, r *http.Request) {
	sandboxID := strings.TrimSpace(chi.URLParam(r, "sandboxID"))
	token := strings.TrimSpace(chi.URLParam(r, "token"))
	if s.logForwardURL == nil || strings.TrimSpace(s.cfg.RunnerLogIngestSigningKey) == "" {
		s.logRejected.Add(1)
		http.Error(w, "log ingestion unavailable", http.StatusServiceUnavailable)
		return
	}
	if sandboxID == "" || token == "" || !s.validSandboxLogToken(sandboxID, token) {
		s.logRejected.Add(1)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	labels, ok := s.backend.SandboxLabels(sandboxID)
	if !ok {
		s.logRejected.Add(1)
		http.Error(w, "sandbox not found", http.StatusNotFound)
		return
	}
	if !s.acquireLogStream(sandboxID) {
		s.logRejected.Add(1)
		http.Error(w, "too many log streams", http.StatusTooManyRequests)
		return
	}
	defer s.releaseLogStream(sandboxID)

	maxBytes := int64(s.cfg.RunnerLogIngestMaxBodyBytes)
	if maxBytes <= 0 {
		maxBytes = 16 << 20
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	target := *s.logForwardURL
	target.RawQuery = ""
	extraFields, ignoreFields := trustedLogHeaders(s.cfg.Environment, s.cfg.RunnerName, sandboxID, labels)

	proxy := httputil.NewSingleHostReverseProxy(&target)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.URL.Path = target.Path
		req.URL.RawPath = target.RawPath
		req.URL.RawQuery = target.RawQuery
		req.Host = target.Host
		req.Header.Del("Authorization")
		req.Header.Del("Cookie")
		req.Header.Del("VL-Extra-Fields")
		req.Header.Del("VL-Ignore-Fields")
		// A nil header value tells ReverseProxy not to append the sandbox IP.
		req.Header["X-Forwarded-For"] = nil
		req.Header.Set("X-Hivy-Log-Source", "sandbox")
		req.Header.Set("VL-Extra-Fields", extraFields)
		req.Header.Set("VL-Ignore-Fields", ignoreFields)
	}
	proxy.ErrorHandler = func(rw http.ResponseWriter, _ *http.Request, _ error) {
		s.logUpstreamErrors.Add(1)
		http.Error(rw, "log storage unavailable", http.StatusBadGateway)
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		if resp.StatusCode >= http.StatusBadRequest {
			s.logUpstreamErrors.Add(1)
		} else {
			s.logAccepted.Add(1)
		}
		return nil
	}
	proxy.ServeHTTP(w, r)
}

func trustedLogHeaders(environment, runnerName, sandboxID string, labels map[string]string) (string, string) {
	fields := map[string]string{
		"source":      "sandbox",
		"environment": environment,
		"runner_id":   runnerName,
		"sandbox_id":  sandboxID,
		"org_id":      labels["org_id"],
		"agent_id":    labels["agent_id"],
		"service":     labels["harness"],
	}
	if fields["service"] == "" {
		fields["service"] = "sandbox"
	}
	keys := make([]string, 0, len(fields))
	for key, value := range fields {
		if strings.TrimSpace(value) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	extraFields := make([]string, 0, len(keys))
	for _, key := range keys {
		extraFields = append(extraFields, key+"="+safeLogFieldValue(fields[key]))
	}
	return strings.Join(extraFields, ","), strings.Join(protectedLogFields, ",")
}

func safeLogFieldValue(value string) string {
	replacer := strings.NewReplacer(",", "_", "=", "_", "\n", "_", "\r", "_")
	return replacer.Replace(strings.TrimSpace(value))
}

func (s *Server) acquireLogStream(sandboxID string) bool {
	s.logStreamsMu.Lock()
	defer s.logStreamsMu.Unlock()
	limit := s.cfg.RunnerLogIngestMaxStreams
	if limit <= 0 {
		limit = 2
	}
	if s.logStreams[sandboxID] >= limit {
		return false
	}
	s.logStreams[sandboxID]++
	return true
}

func (s *Server) releaseLogStream(sandboxID string) {
	s.logStreamsMu.Lock()
	defer s.logStreamsMu.Unlock()
	if s.logStreams[sandboxID] <= 1 {
		delete(s.logStreams, sandboxID)
		return
	}
	s.logStreams[sandboxID]--
}

func (s *Server) logIngestMetrics(w http.ResponseWriter, _ *http.Request) {
	s.logStreamsMu.Lock()
	activeStreams := 0
	for _, count := range s.logStreams {
		activeStreams += count
	}
	s.logStreamsMu.Unlock()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = fmt.Fprintf(w, "# TYPE hivy_runner_sandbox_log_requests_total counter\n")
	_, _ = fmt.Fprintf(w, "hivy_runner_sandbox_log_requests_total{result=\"accepted\"} %d\n", s.logAccepted.Load())
	_, _ = fmt.Fprintf(w, "hivy_runner_sandbox_log_requests_total{result=\"rejected\"} %d\n", s.logRejected.Load())
	_, _ = fmt.Fprintf(w, "hivy_runner_sandbox_log_requests_total{result=\"upstream_error\"} %d\n", s.logUpstreamErrors.Load())
	_, _ = fmt.Fprintf(w, "# TYPE hivy_runner_sandbox_log_streams gauge\n")
	_, _ = fmt.Fprintf(w, "hivy_runner_sandbox_log_streams %d\n", activeStreams)
}
