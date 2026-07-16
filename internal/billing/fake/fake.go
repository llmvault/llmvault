package fake

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/billing"
)

type Provider struct {
	mu sync.Mutex

	ProviderName      string
	NextCreateError   error
	NextResolveError  error
	NextResolveResult *billing.DepositResult
	deposits          []billing.DepositIntent
}

func New(name string) *Provider { return &Provider{ProviderName: name} }

func (p *Provider) Name() string {
	if p.ProviderName == "" {
		return "fake"
	}
	return p.ProviderName
}

func (p *Provider) CreateDeposit(_ context.Context, intent billing.DepositIntent) (*billing.DepositSession, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deposits = append(p.deposits, intent)
	if p.NextCreateError != nil {
		return nil, p.NextCreateError
	}
	ref := "ref_" + uuid.NewString()
	return &billing.DepositSession{URL: "https://example.test/pay/" + ref, AccessCode: "access_" + ref, Reference: ref}, nil
}

func (p *Provider) ResolveDeposit(_ context.Context, req billing.ResolveDepositRequest) (*billing.DepositResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.NextResolveError != nil {
		return nil, p.NextResolveError
	}
	if p.NextResolveResult != nil {
		copy := *p.NextResolveResult
		return &copy, nil
	}
	return &billing.DepositResult{Status: billing.PaymentPaid, Reference: req.Reference}, nil
}

func (p *Provider) Deposits() []billing.DepositIntent {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]billing.DepositIntent, len(p.deposits))
	copy(out, p.deposits)
	return out
}
