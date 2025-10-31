package auth

// SessionData represents the authenticated session context for a request
type SessionData struct {
	UserID     string `json:"user_id"`
	Email      string `json:"email"`
	IsAdmin    bool   `json:"is_admin"`
	AuthMethod string `json:"auth_method"` // "web", "cli"
}
