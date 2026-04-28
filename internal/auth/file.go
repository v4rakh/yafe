package auth

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
)

// FileAuthenticator loads users from a file.
type FileAuthenticator struct {
	users []*User
}

// NewFileAuthenticator creates an authenticator from a file.
// File format: user:$bcrypt_hash:role1,role2
// Lines starting with # are comments, empty lines are ignored.
func NewFileAuthenticator(path string) (*FileAuthenticator, error) {
	//gosec:disable G304 -- Explicitly allowed to use any location
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening auth file: %w", err)
	}
	defer file.Close()

	var users []*User
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		user, err := parseLine(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNum, err)
		}

		// Check for duplicate usernames
		for _, existing := range users {
			if existing.Name == user.Name {
				return nil, fmt.Errorf("line %d: duplicate user: %s", lineNum, user.Name)
			}
		}

		users = append(users, user)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading auth file: %w", err)
	}

	return &FileAuthenticator{users: users}, nil
}

func parseLine(line string) (*User, error) {
	// Format: user:$bcrypt_hash:role1,role2
	parts := strings.SplitN(line, ":", 3)
	if len(parts) < 2 {
		return nil, errors.New("invalid format, expected user:hash[:roles]")
	}

	name := strings.TrimSpace(parts[0])
	if name == "" {
		return nil, errors.New("empty username")
	}

	hash := strings.TrimSpace(parts[1])
	if hash == "" {
		return nil, errors.New("empty hash")
	}

	// Validate it looks like a bcrypt hash
	if !strings.HasPrefix(hash, "$2") {
		return nil, errors.New("invalid hash format, must be bcrypt (starts with $2)")
	}

	var roles []Role
	if len(parts) == 3 && parts[2] != "" {
		var err error
		roles, err = ParseRoles(parts[2])
		if err != nil {
			return nil, err
		}
	}

	return &User{
		Name:  name,
		Hash:  []byte(hash),
		Roles: roles,
	}, nil
}

// Authenticate finds a user by API key.
func (a *FileAuthenticator) Authenticate(key string) *User {
	for _, user := range a.users {
		if user.ValidateKey(key) {
			return user
		}
	}
	return nil
}

// InlineAuthenticator authenticates a single user configured via flags.
type InlineAuthenticator struct {
	user *User
}

// NewInlineAuthenticator creates an authenticator for a single user.
func NewInlineAuthenticator(name, hash, roles string) (*InlineAuthenticator, error) {
	if name == "" {
		return nil, errors.New("username required")
	}
	if hash == "" {
		return nil, errors.New("hash required")
	}
	if !strings.HasPrefix(hash, "$2") {
		return nil, errors.New("invalid hash format, must be bcrypt (starts with $2)")
	}

	parsedRoles, err := ParseRoles(roles)
	if err != nil {
		return nil, err
	}

	return &InlineAuthenticator{
		user: &User{
			Name:  name,
			Hash:  []byte(hash),
			Roles: parsedRoles,
		},
	}, nil
}

// Authenticate validates the API key.
func (a *InlineAuthenticator) Authenticate(key string) *User {
	if a.user.ValidateKey(key) {
		return a.user
	}
	return nil
}

// MultiAuthenticator combines multiple authenticators.
type MultiAuthenticator struct {
	authenticators []Authenticator
}

// NewMultiAuthenticator creates an authenticator that tries multiple sources.
func NewMultiAuthenticator(auths ...Authenticator) *MultiAuthenticator {
	return &MultiAuthenticator{authenticators: auths}
}

// Authenticate tries each authenticator in order.
func (a *MultiAuthenticator) Authenticate(key string) *User {
	for _, auth := range a.authenticators {
		if user := auth.Authenticate(key); user != nil {
			return user
		}
	}
	return nil
}
