package auth

import (
	"context"
	"slices"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// ParseRoles parses a comma-separated list of roles.
func ParseRoles(s string) ([]Role, error) {
	if s == "" {
		return nil, nil
	}

	parts := strings.Split(s, ",")
	roles := make([]Role, 0, len(parts))
	seen := make(map[Role]bool)

	for _, p := range parts {
		role, err := ParseRole(strings.TrimSpace(p))
		if err != nil {
			return nil, err
		}
		if !seen[role] {
			roles = append(roles, role)
			seen[role] = true
		}
	}

	return roles, nil
}

// User represents an authenticated user.
type User struct {
	Name  string
	Hash  []byte // bcrypt hash of the API key
	Roles []Role
}

// HasRole checks if user has a specific role.
func (u *User) HasRole(r Role) bool {
	return slices.Contains(u.Roles, r)
}

// ValidateKey checks if the provided key matches the user's hash.
func (u *User) ValidateKey(key string) bool {
	err := bcrypt.CompareHashAndPassword(u.Hash, []byte(key))
	return err == nil
}

// Authenticator validates API keys and returns the associated user.
type Authenticator interface {
	// Authenticate returns user if key is valid, nil otherwise.
	Authenticate(key string) *User
}

// HashKey generates a bcrypt hash for an API key.
func HashKey(key string) ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte(key), bcrypt.DefaultCost)
}

// contextKey is used for storing user in request context.
type contextKey struct{}

var userContextKey = contextKey{}

// WithUser returns a new context with the user attached.
func WithUser(ctx context.Context, user *User) context.Context {
	return context.WithValue(ctx, userContextKey, user)
}

// GetUserFromContext retrieves the authenticated user from context.
func GetUserFromContext(ctx context.Context) *User {
	user, _ := ctx.Value(userContextKey).(*User)
	return user
}
