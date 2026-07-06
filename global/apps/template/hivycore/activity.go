// DO NOT EDIT — hivycore is the Hivy platform core. See doc.go.

package hivycore

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// activityDebounce caps activity pings at one per window so a busy app never
// floods the API.
const activityDebounce = time.Minute

// activityReporter pings the platform (debounced, fire-and-forget) so it holds
// off auto-sleep. Nil when unconfigured (docker/local).
type activityReporter struct {
	url    string
	secret string
	client *http.Client
	log    *slog.Logger

	mu   sync.Mutex
	last time.Time
}

func newActivityReporter(url, secret string, log *slog.Logger) *activityReporter {
	if url == "" {
		return nil
	}
	return &activityReporter{
		url:    url,
		secret: secret,
		client: &http.Client{Timeout: 5 * time.Second},
		log:    log,
	}
}

// ping reports at most once per debounce window, never blocking the request.
func (a *activityReporter) ping() {
	if a == nil {
		return
	}
	now := time.Now()
	a.mu.Lock()
	if now.Sub(a.last) < activityDebounce {
		a.mu.Unlock()
		return
	}
	a.last = now
	a.mu.Unlock()
	go a.send()
}

func (a *activityReporter) send() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.url, nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+a.secret)
	resp, err := a.client.Do(req)
	if err != nil {
		a.log.Warn("activity report failed", "error", err.Error())
		return
	}
	_ = resp.Body.Close()
}
