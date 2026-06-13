package control

import (
	"fmt"
	"html"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/usehivy/hivy/internal/microsandbox/api"
	"github.com/usehivy/hivy/internal/microsandbox/httpx"
	"github.com/usehivy/hivy/internal/microsandbox/model"
	"github.com/usehivy/hivy/internal/microsandbox/security"
)

type previewClaims struct {
	SandboxID string `json:"sandbox_id"`
	jwt.RegisteredClaims
}

type runtimeEndpointClaims struct {
	SandboxID string `json:"sandbox_id"`
	Port      int    `json:"port"`
	Purpose   string `json:"purpose"`
	jwt.RegisteredClaims
}

type previewSessionRequest struct {
	SandboxID  string `json:"sandbox_id"`
	Port       int    `json:"port"`
	TTLSeconds int    `json:"ttl_seconds"`
}

func (s *Server) createPreviewSession(w http.ResponseWriter, r *http.Request) {
	var req previewSessionRequest
	if err := httpx.Decode(r, &req); err != nil || req.SandboxID == "" || req.Port <= 0 {
		httpx.JSON(w, http.StatusBadRequest, api.ErrorResponse{Error: "sandbox_id and port are required"})
		return
	}
	var sb model.Sandbox
	if err := s.db.First(&sb, "id = ?", req.SandboxID).Error; err != nil {
		httpx.JSON(w, http.StatusNotFound, api.ErrorResponse{Error: "sandbox not found"})
		return
	}
	ttl := s.cfg.PreviewCookieTTL
	if req.TTLSeconds > 0 {
		ttl = time.Duration(req.TTLSeconds) * time.Second
	}
	token, err := s.signPreviewToken(sb.ID, ttl)
	if err != nil {
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to sign preview token"})
		return
	}
	previewURL := fmt.Sprintf("https://%d-%s.%s?t=%s", req.Port, sb.ID, s.cfg.PreviewBaseDomain, url.QueryEscape(token))
	httpx.JSON(w, http.StatusOK, map[string]string{"token": token, "url": previewURL})
}

func (s *Server) previewProxy() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		port, sandboxID, ok := parsePreviewHost(r.Host)
		if !ok {
			http.NotFound(w, r)
			return
		}
		if token := r.URL.Query().Get("rt"); token != "" && s.validRuntimeToken(token, sandboxID, port) {
			clean := *r.URL
			q := clean.Query()
			q.Del("rt")
			clean.RawQuery = q.Encode()
			s.proxyPreviewRequest(w, r, sandboxID, port, clean.RawQuery)
			return
		}
		if token := r.URL.Query().Get("t"); token != "" && s.validPreviewToken(token, sandboxID) {
			s.setPreviewCookie(w, sandboxID)
			clean := *r.URL
			q := clean.Query()
			q.Del("t")
			clean.RawQuery = q.Encode()
			http.Redirect(w, r, clean.String(), http.StatusFound)
			return
		}
		if !s.previewCookieValid(r, sandboxID) {
			http.Redirect(w, r, "/preview/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusFound)
			return
		}
		s.proxyPreviewRequest(w, r, sandboxID, port, r.URL.RawQuery)
	})
}

func (s *Server) proxyPreviewRequest(w http.ResponseWriter, r *http.Request, sandboxID string, port int, rawQuery string) {
	target, err := s.previewTarget(r.Context(), sandboxID, port)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.URL.Path = strings.TrimRight(target.Path, "/") + r.URL.Path
		req.URL.RawQuery = rawQuery
		req.Host = target.Host
		req.Header.Set("X-Hivy-Sandbox-ID", sandboxID)
		req.Header.Set("X-Hivy-Preview-Port", strconv.Itoa(port))
		req.Header.Set("Authorization", "Bearer "+s.cfg.RunnerAPIToken)
	}
	proxy.ServeHTTP(w, r)
}

func (s *Server) previewLoginForm(w http.ResponseWriter, r *http.Request) {
	_, sandboxID, ok := parsePreviewHost(r.Host)
	if !ok {
		http.NotFound(w, r)
		return
	}
	next := r.URL.Query().Get("next")
	if next == "" {
		next = "/"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, `<!doctype html><html><head><title>Preview locked</title><meta name="viewport" content="width=device-width,initial-scale=1"></head><body style="font-family:system-ui,sans-serif;max-width:420px;margin:12vh auto;padding:24px"><h1>Preview locked</h1><form method="post" action="/preview/login"><input type="hidden" name="sandbox_id" value="%s"><input type="hidden" name="next" value="%s"><label>Password<br><input name="password" type="password" autofocus style="width:100%%;font-size:16px;padding:10px;margin-top:6px"></label><button style="margin-top:14px;padding:10px 14px">Continue</button></form></body></html>`,
		html.EscapeString(sandboxID), html.EscapeString(next))
}

func (s *Server) previewLoginPost(w http.ResponseWriter, r *http.Request) {
	_, sandboxID, ok := parsePreviewHost(r.Host)
	if !ok || sandboxID != r.FormValue("sandbox_id") {
		http.NotFound(w, r)
		return
	}
	var sb model.Sandbox
	if err := s.db.First(&sb, "id = ?", sandboxID).Error; err != nil {
		http.NotFound(w, r)
		return
	}
	var secret model.OrgPreviewSecret
	passwordOK := false
	if err := s.db.First(&secret, "org_id = ?", sb.OrgID).Error; err == nil {
		password, err := security.DecryptString(s.cfg.PreviewPasswordKey, secret.PasswordCiphertext)
		passwordOK = err == nil && security.ConstantTimeStringEqual(r.FormValue("password"), password)
	}
	if !passwordOK {
		http.Error(w, "invalid password", http.StatusUnauthorized)
		return
	}
	s.setPreviewCookie(w, sandboxID)
	next := r.FormValue("next")
	if next == "" || !strings.HasPrefix(next, "/") {
		next = "/"
	}
	http.Redirect(w, r, next, http.StatusFound)
}

func (s *Server) previewTarget(_ any, sandboxID string, port int) (*url.URL, error) {
	var sb model.Sandbox
	if err := s.db.First(&sb, "id = ?", sandboxID).Error; err != nil {
		return nil, fmt.Errorf("sandbox not found")
	}
	var runner model.Runner
	if err := s.db.First(&runner, "id = ?", sb.RunnerID).Error; err != nil {
		return nil, fmt.Errorf("runner not found")
	}
	target := fmt.Sprintf("%s/proxy/%s/%d", strings.TrimRight(runner.APIURL, "/"), sandboxID, port)
	return url.Parse(target)
}

func parsePreviewHost(host string) (int, string, bool) {
	host = strings.Split(host, ":")[0]
	first := strings.Split(host, ".")[0]
	parts := strings.SplitN(first, "-", 2)
	if len(parts) != 2 {
		return 0, "", false
	}
	port, err := strconv.Atoi(parts[0])
	if err != nil || port <= 0 || port > 65535 {
		return 0, "", false
	}
	return port, parts[1], true
}

func (s *Server) signPreviewToken(sandboxID string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := previewClaims{
		SandboxID: sandboxID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   sandboxID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.cfg.PreviewJWTSecret))
}

func (s *Server) signRuntimeToken(sandboxID string, port int, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := runtimeEndpointClaims{
		SandboxID: sandboxID,
		Port:      port,
		Purpose:   "runtime_endpoint",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   sandboxID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.cfg.PreviewJWTSecret))
}

func (s *Server) validPreviewToken(raw, sandboxID string) bool {
	token, err := jwt.ParseWithClaims(raw, &previewClaims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(s.cfg.PreviewJWTSecret), nil
	})
	if err != nil || !token.Valid {
		return false
	}
	claims, ok := token.Claims.(*previewClaims)
	return ok && claims.SandboxID == sandboxID
}

func (s *Server) validRuntimeToken(raw, sandboxID string, port int) bool {
	token, err := jwt.ParseWithClaims(raw, &runtimeEndpointClaims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(s.cfg.PreviewJWTSecret), nil
	})
	if err != nil || !token.Valid {
		return false
	}
	claims, ok := token.Claims.(*runtimeEndpointClaims)
	return ok && claims.Purpose == "runtime_endpoint" && claims.SandboxID == sandboxID && claims.Port == port
}

func (s *Server) setPreviewCookie(w http.ResponseWriter, sandboxID string) {
	token, _ := s.signPreviewToken(sandboxID, s.cfg.PreviewCookieTTL)
	cookie := &http.Cookie{
		Name:     "microsandbox_preview",
		Value:    token,
		Path:     "/",
		MaxAge:   int(s.cfg.PreviewCookieTTL.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
	if s.cfg.PreviewCookieDomain != "" {
		cookie.Domain = s.cfg.PreviewCookieDomain
	}
	http.SetCookie(w, cookie)
}

func (s *Server) previewCookieValid(r *http.Request, sandboxID string) bool {
	cookie, err := r.Cookie("microsandbox_preview")
	if err != nil {
		return false
	}
	return s.validPreviewToken(cookie.Value, sandboxID)
}
