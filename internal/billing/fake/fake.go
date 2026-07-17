package fake

import (
	"context"
	"sync"

	"github.com/usehivy/hivy/internal/billing"
)

type Provider struct {
	mu sync.Mutex

	ProviderName      string
	NextCreateError   error
	NextResolveError  error
	NextResolveResult *billing.DepositResult
	deposits          []billing.DepositIntent
	savedCharges      []billing.SavedPaymentCharge
}

func (p *Provider) ChargeSavedPayment(_ context.Context, charge billing.SavedPaymentCharge) (*billing.DepositSession, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.savedCharges = append(p.savedCharges, charge)
	if p.NextCreateError != nil {
		return nil, p.NextCreateError
	}
	return &billing.DepositSession{Reference: charge.PurchaseID.String()}, nil
}

func New(name string) *Provider { return &Provider{ProviderName: name} }

func (p *Provider) Name() string {
	if p.ProviderName == "" {
		return "fake"
	}
	return p.ProviderName
}

func (p *Provider) SavedCharges() []billing.SavedPaymentCharge {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]billing.SavedPaymentCharge, len(p.savedCharges))
	copy(out, p.savedCharges)
	return out
}

func (p *Provider) CreateDeposit(_ context.Context, intent billing.DepositIntent) (*billing.DepositSession, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deposits = append(p.deposits, intent)
	if p.NextCreateError != nil {
		return nil, p.NextCreateError
	}
	ref := intent.PurchaseID.String()
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
		if copy.Reference == "" {
			copy.Reference = req.Reference
		}
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
