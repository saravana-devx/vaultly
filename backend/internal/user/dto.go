package user

type RegisterRequest struct {
	Name     string `json:"name" binding:"required"`
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required,min=8,max=72"`
	OrgName  string `json:"org_name"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type AuthResponse struct {
	AccessToken string  `json:"access_token"`
	ExpiresIn   int     `json:"expires_in"`
	User        Profile `json:"user"`
}

type LoginMFARequest struct {
	Challenge string `json:"challenge" binding:"required"`
	Code      string `json:"code" binding:"required,max=32"`
}

type MFACodeRequest struct {
	Code string `json:"code" binding:"required,max=32"`
}

type MFADisableRequest struct {
	Password string `json:"password" binding:"required"`
	Code     string `json:"code" binding:"required,max=32"`
}

// MFAChallengeResponse is what login returns when a second factor is needed.
type MFAChallengeResponse struct {
	MFARequired bool   `json:"mfa_required"`
	Challenge   string `json:"challenge"`
}

type RecoveryCodesResponse struct {
	RecoveryCodes []string `json:"recovery_codes"`
}
