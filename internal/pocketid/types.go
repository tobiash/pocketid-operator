package pocketid

type OIDCClient struct {
	ID           string   `json:"id,omitempty"`
	Name         string   `json:"name"`
	CallbackURLs []string `json:"callbackURLs"`
	IsPublic     bool     `json:"isPublic"`
	PkceEnabled  bool     `json:"pkceEnabled"`
	HasLogo      bool     `json:"hasLogo"`
}

type OIDCClientCreate struct {
	ID           string   `json:"id,omitempty"`
	Name         string   `json:"name"`
	CallbackURLs []string `json:"callbackURLs"`
	IsPublic     bool     `json:"isPublic"`
	PkceEnabled  bool     `json:"pkceEnabled"`
}

type User struct {
	ID          string  `json:"id,omitempty"`
	Username    string  `json:"username"`
	Email       *string `json:"email,omitempty"`
	FirstName   string  `json:"firstName"`
	LastName    *string `json:"lastName,omitempty"`
	DisplayName string  `json:"displayName"`
	IsAdmin     bool    `json:"isAdmin"`
	Disabled    bool    `json:"disabled,omitempty"`
	Locale      *string `json:"locale,omitempty"`
	UserGroups  []struct {
		ID           string `json:"id"`
		FriendlyName string `json:"friendlyName"`
		Name         string `json:"name"`
	} `json:"userGroups,omitempty"`
}

type UserGroup struct {
	ID           string `json:"id,omitempty"`
	Name         string `json:"name"`
	FriendlyName string `json:"friendlyName,omitempty"`
}

type UserGroupCreate struct {
	Name         string `json:"name"`
	FriendlyName string `json:"friendlyName"`
}

type OnboardingResponse struct {
	Token string `json:"token"`
}

type PaginatedResponse[T any] struct {
	Data       []T         `json:"data"`
	Pagination *Pagination `json:"pagination,omitempty"`
}

type Pagination struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	TotalPages int `json:"totalPages"`
	TotalItems int `json:"totalItems"`
}
