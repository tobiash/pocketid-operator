package pocketid

// OIDCClient represents an OIDC client in PocketID
type OIDCClient struct {
	ID           string   `json:"id,omitempty"`
	Name         string   `json:"name"`
	ClientID     string   `json:"clientId,omitempty"`
	ClientSecret string   `json:"clientSecret,omitempty"` // Only returned on creation/generation
	RedirectURIs []string `json:"redirectUris"`
	IsPublic     bool     `json:"isPublic"`
	Scopes       []string `json:"scopes,omitempty"`
}

// User represents a user in PocketID
type User struct {
	ID          string   `json:"id,omitempty"`
	Username    string   `json:"username"`
	Email       string   `json:"email,omitempty"`
	FirstName   string   `json:"firstName"`
	LastName    string   `json:"lastName,omitempty"`
	DisplayName string   `json:"displayName"`
	IsAdmin     bool     `json:"isAdmin"`
	Disabled    bool     `json:"disabled,omitempty"`
	Locale      string   `json:"locale,omitempty"`
	GroupIDs    []string `json:"groupIds,omitempty"`
}

// UserGroup represents a user group in PocketID
type UserGroup struct {
	ID           string        `json:"id,omitempty"`
	Name         string        `json:"name"`
	FriendlyName string        `json:"friendlyName,omitempty"`
	IsDefault    bool          `json:"isDefault,omitempty"`
	CustomClaims []CustomClaim `json:"customClaims,omitempty"`
}

// CustomClaim represents a custom JWT claim
type CustomClaim struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// OnboardingResponse contains the one-time access link
type OnboardingResponse struct {
	OneTimeAccessLink string `json:"oneTimeAccessLink"`
}
