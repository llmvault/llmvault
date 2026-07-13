package handler

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	OrgID    string `json:"org_id,omitempty"` // optional: scope token to a specific org
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
	OrgID        string `json:"org_id,omitempty"` // optional: switch org
}

type logoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type authResponse struct {
	AccessToken  string         `json:"access_token"`
	RefreshToken string         `json:"refresh_token"`
	ExpiresIn    int            `json:"expires_in"` // seconds
	User         userResponse   `json:"user"`
	Orgs         []orgMemberDTO `json:"orgs"`
}

type userResponse struct {
	ID             string `json:"id"`
	Email          string `json:"email"`
	Name           string `json:"name"`
	AvatarURL      string `json:"avatar_url,omitempty"`
	EmailConfirmed bool   `json:"email_confirmed"`
}

type updateProfileRequest struct {
	Name      *string `json:"name,omitempty"`
	Email     *string `json:"email,omitempty"`
	AvatarURL *string `json:"avatar_url,omitempty"`
}

type orgMemberDTO struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Role           string   `json:"role"`
	Plan           *planDTO `json:"plan,omitempty"`
	Credits        *int64   `json:"credits,omitempty"`
	BYOK           bool     `json:"byok"`
	LogoURL        string   `json:"logo_url,omitempty"`
	OnboardingStep string   `json:"onboarding_step"`
}

// planDTO is the customer-facing shape of a billing plan. Returned both as a
// nested field on org DTOs and as the element type of GET /v1/plans.
type planDTO struct {
	Slug           string   `json:"slug"`
	Name           string   `json:"name"`
	Provider       string   `json:"provider,omitempty"`
	Features       []string `json:"features,omitempty"`
	MonthlyCredits int64    `json:"monthly_credits"`
	WelcomeCredits int64    `json:"welcome_credits"`
	PriceCents     int64    `json:"price_cents"`
	Currency       string   `json:"currency"`
}

type meResponse struct {
	User userResponse   `json:"user"`
	Orgs []orgMemberDTO `json:"orgs"`
}

type statusResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}
