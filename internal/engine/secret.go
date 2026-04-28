package engine

import (
	"fmt"
	"os"
	"strings"
)

// SecretSource defines where to read a secret from.
type SecretSource struct {
	Env  string `yaml:"env"`  // environment variable name
	File string `yaml:"file"` // file path
}

// SecretDeclaration defines a secret requirement.
type SecretDeclaration struct {
	Name string       `yaml:"name"` // secret identifier
	From SecretSource `yaml:"from"` // source configuration
}

// SecretManager handles secret resolution and masking.
type SecretManager struct {
	secrets map[string]string
}

// NewSecretManager creates a new secret manager.
func NewSecretManager() *SecretManager {
	return &SecretManager{
		secrets: make(map[string]string),
	}
}

// Load resolves and loads secrets from their declared sources.
func (m *SecretManager) Load(declarations []SecretDeclaration) error {
	for _, decl := range declarations {
		value, err := m.resolveSecret(decl)
		if err != nil {
			return fmt.Errorf("secret %q: %w", decl.Name, err)
		}
		m.secrets[decl.Name] = value
	}
	return nil
}

// resolveSecret reads the secret value from its source.
func (m *SecretManager) resolveSecret(decl SecretDeclaration) (string, error) {
	// Try environment variable first
	if decl.From.Env != "" {
		value, ok := os.LookupEnv(decl.From.Env)
		if ok {
			return value, nil
		}
		// If file is also specified, try that as fallback
		if decl.From.File == "" {
			return "", fmt.Errorf("%w: %s", ErrSecretEnvNotSet, decl.From.Env)
		}
	}

	// Try file
	if decl.From.File != "" {
		data, err := os.ReadFile(decl.From.File)
		if err != nil {
			return "", fmt.Errorf("%w: %s: %w", ErrSecretFileRead, decl.From.File, err)
		}
		return strings.TrimSpace(string(data)), nil
	}

	return "", ErrSecretSourceEmpty
}

// Get retrieves a secret value by name.
func (m *SecretManager) Get(name string) (string, bool) {
	value, ok := m.secrets[name]
	return value, ok
}

// Has checks if a secret exists.
func (m *SecretManager) Has(name string) bool {
	_, ok := m.secrets[name]
	return ok
}

// Names returns all secret names.
func (m *SecretManager) Names() []string {
	names := make([]string, 0, len(m.secrets))
	for name := range m.secrets {
		names = append(names, name)
	}
	return names
}

// Mask replaces all secret values in the input string with the mask.
func (m *SecretManager) Mask(input string) string {
	result := input
	for _, value := range m.secrets {
		if value != "" {
			result = strings.ReplaceAll(result, value, secretMask)
		}
	}
	return result
}

// Clone creates a copy of the secret manager.
func (m *SecretManager) Clone() *SecretManager {
	clone := NewSecretManager()
	for name, value := range m.secrets {
		clone.secrets[name] = value
	}
	return clone
}

// Clear zeros out all secret values.
func (m *SecretManager) Clear() {
	for name := range m.secrets {
		m.secrets[name] = ""
	}
	m.secrets = nil
}

// Set sets a secret value directly. Primarily used for testing.
func (m *SecretManager) Set(name, value string) {
	m.secrets[name] = value
}
