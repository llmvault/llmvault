package runner

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
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
	r.Body = newRedactingJournalBody(r.Body, maxBytes)
	// Redaction can change a field's encoded length, so the original
	// Content-Length must not be forwarded.
	r.ContentLength = -1
	r.Header.Del("Content-Length")
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
		"source":                  "sandbox",
		"environment":             environment,
		"runner_id":               runnerName,
		"sandbox_id":              sandboxID,
		"org_id":                  labels["org_id"],
		"agent_id":                labels["agent_id"],
		"session_id":              labels["session_id"],
		"provisioning_attempt_id": labels["provisioning_attempt_id"],
		"trace_id":                labels["trace_id"],
		"service":                 labels["harness"],
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

const redactedEnvironmentValue = "[REDACTED]"

type redactingJournalBody struct {
	reader      *bufio.Reader
	closer      io.Closer
	maxFieldLen int64
	pending     []byte
	pendingErr  error
}

func newRedactingJournalBody(body io.ReadCloser, maxFieldLen int64) io.ReadCloser {
	return &redactingJournalBody{
		reader:      bufio.NewReader(body),
		closer:      body,
		maxFieldLen: maxFieldLen,
	}
}

func (r *redactingJournalBody) Read(p []byte) (int, error) {
	for len(r.pending) == 0 {
		if r.pendingErr != nil {
			return 0, r.pendingErr
		}
		r.pending, r.pendingErr = r.readField()
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

func (r *redactingJournalBody) Close() error {
	return r.closer.Close()
}

func (r *redactingJournalBody) readField() ([]byte, error) {
	line, err := r.reader.ReadBytes('\n')
	if err != nil {
		return line, err
	}
	if len(line) == 1 {
		return line, nil
	}

	fieldLine := line[:len(line)-1]
	if equals := bytes.IndexByte(fieldLine, '='); equals >= 0 {
		if string(fieldLine[:equals]) != "MESSAGE" {
			return line, nil
		}
		message := redactEnvironmentAssignment(fieldLine[equals+1:])
		out := make([]byte, 0, len("MESSAGE=")+len(message)+1)
		out = append(out, "MESSAGE="...)
		out = append(out, message...)
		out = append(out, '\n')
		return out, nil
	}

	// Journal export encodes fields containing non-printable data as a field
	// name, an unsigned little-endian length, the raw value, and a newline.
	var encodedLength [8]byte
	if _, err := io.ReadFull(r.reader, encodedLength[:]); err != nil {
		return line, err
	}
	valueLength := binary.LittleEndian.Uint64(encodedLength[:])
	if valueLength > uint64(r.maxFieldLen) {
		return nil, fmt.Errorf("journal field exceeds maximum request size")
	}
	value := make([]byte, valueLength)
	if _, err := io.ReadFull(r.reader, value); err != nil {
		return nil, err
	}
	terminator, err := r.reader.ReadByte()
	if err != nil {
		return nil, err
	}
	if terminator != '\n' {
		return nil, fmt.Errorf("journal binary field is missing terminator")
	}
	if string(fieldLine) == "MESSAGE" {
		value = redactEnvironmentAssignment(value)
		binary.LittleEndian.PutUint64(encodedLength[:], uint64(len(value)))
	}
	out := make([]byte, 0, len(line)+len(encodedLength)+len(value)+1)
	out = append(out, line...)
	out = append(out, encodedLength[:]...)
	out = append(out, value...)
	out = append(out, '\n')
	return out, nil
}

func redactEnvironmentAssignment(message []byte) []byte {
	keyStart := 0
	for keyStart < len(message) && (message[keyStart] == ' ' || message[keyStart] == '\t') {
		keyStart++
	}
	equals := bytes.IndexByte(message[keyStart:], '=')
	if equals <= 0 {
		return message
	}
	equals += keyStart
	for _, ch := range message[keyStart:equals] {
		if ch != '_' && !isASCIILetterOrDigit(ch) {
			return message
		}
	}
	first := message[keyStart]
	if first != '_' && !isASCIILetter(first) {
		return message
	}
	if equals == len(message)-1 {
		return message
	}
	out := make([]byte, 0, equals+1+len(redactedEnvironmentValue))
	out = append(out, message[:equals+1]...)
	out = append(out, redactedEnvironmentValue...)
	return out
}

func isASCIILetter(ch byte) bool {
	return ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z'
}

func isASCIILetterOrDigit(ch byte) bool {
	return isASCIILetter(ch) || ch >= '0' && ch <= '9'
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
