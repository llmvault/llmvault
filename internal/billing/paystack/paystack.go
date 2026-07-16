// Package paystack implements one-time credit deposits through Paystack.
package paystack

import "github.com/usehivy/hivy/internal/billing"

// Name is the stable provider slug stored on credit purchases.
const Name = "paystack"

type Provider struct {
	cfg    Config
	client *client
}

func New(cfg Config) *Provider {
	return &Provider{cfg: cfg, client: newClient(cfg.SecretKey)}
}

func (p *Provider) Name() string { return Name }

var _ billing.Provider = (*Provider)(nil)
