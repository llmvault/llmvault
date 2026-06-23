package sandbox

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/keyedlock"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// Orchestrator manages sandbox lifecycle — creating, starting, stopping sandboxes
// and providing runtime clients to talk to them.
type Orchestrator struct {
	db                *gorm.DB
	provider          Provider
	encKey            *crypto.SymmetricKey
	cfg               *config.Config
	warmPool          *WarmPool
	reconcileWarmPool func(context.Context, string, WarmPoolProfile) error
	pushRuntimeConfig func(context.Context, *model.Sandbox, *agentruntime.ProxyTokenResult) error

	// lastActiveTouch debounces the last_active_at write done on every runtime-client
	// fetch so the hot path does not issue a DB UPDATE per request.
	lastActiveTouchMu sync.Mutex
	lastActiveTouch   map[uuid.UUID]time.Time

	lifecycle keyedlock.Locker
}

func NewOrchestrator(db *gorm.DB, provider Provider, encKey *crypto.SymmetricKey, cfg *config.Config) *Orchestrator {
	return &Orchestrator{
		db:       db,
		provider: provider,
		encKey:   encKey,
		cfg:      cfg,
		warmPool: NewWarmPool(db, provider, encKey, cfg),
	}
}

func (o *Orchestrator) WarmPool() *WarmPool {
	return o.warmPool
}

func (o *Orchestrator) SetWarmPoolReconciler(fn func(context.Context, string, WarmPoolProfile) error) {
	o.reconcileWarmPool = fn
}

func (o *Orchestrator) SetAgentRuntimeConfigPusher(fn func(context.Context, *model.Sandbox, *agentruntime.ProxyTokenResult) error) {
	o.pushRuntimeConfig = fn
}

func (o *Orchestrator) pushAgentRuntimeConfig(ctx context.Context, sb *model.Sandbox, reason string, proxyToken *agentruntime.ProxyTokenResult) error {
	if o.pushRuntimeConfig == nil {
		return nil
	}
	if err := o.pushRuntimeConfig(ctx, sb, proxyToken); err != nil {
		return fmt.Errorf("push agent runtime config after %s: %w", reason, err)
	}
	return nil
}

func (o *Orchestrator) enqueueWarmPoolReconcile(ctx context.Context, profile WarmPoolProfile) {
	if o.reconcileWarmPool == nil {
		return
	}
	if err := o.reconcileWarmPool(ctx, o.provider.ID(), profile); err != nil {
		logging.Capture(ctx, fmt.Errorf("enqueue warm pool reconcile: %w", err))
	}
}

func (o *Orchestrator) ProviderID() string {
	return o.providerID()
}

func (o *Orchestrator) Config() *config.Config {
	if o == nil {
		return nil
	}
	return o.cfg
}

func (o *Orchestrator) GetRuntimeClient(ctx context.Context, sb *model.Sandbox) (*agentruntime.Client, error) {
	if err := o.ensureSandboxProvider(sb); err != nil {
		return nil, err
	}
	apiKey, err := o.encKey.DecryptString(sb.EncryptedRuntimeSecret)
	if err != nil {
		return nil, fmt.Errorf("decrypting runtime secret: %w", err)
	}
	if o.provider.ID() == ProviderMicrosandbox {
		if sb.RuntimeURL == "" {
			if err := o.RefreshAgentSandboxURL(ctx, sb); err != nil {
				return nil, fmt.Errorf("refreshing runtime URL: %w", err)
			}
		}
		return agentruntime.NewClient(sb.RuntimeURL, apiKey), nil
	}
	if _, err := o.EnsureSandboxActive(ctx, sb); err != nil {
		return nil, fmt.Errorf("ensuring sandbox active: %w", err)
	}
	if o.needsURLRefresh(sb) {
		if err := o.RefreshAgentSandboxURL(ctx, sb); err != nil {
			return nil, fmt.Errorf("refreshing runtime URL: %w", err)
		}
	}
	o.touchLastActive(ctx, sb)
	return agentruntime.NewClient(sb.RuntimeURL, apiKey), nil
}

func (o *Orchestrator) AgentDriveUploadURL(agentID uuid.UUID) string {
	return agentDriveUploadURL(o.cfg, agentID)
}
