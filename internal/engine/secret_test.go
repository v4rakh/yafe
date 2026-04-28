package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSecretManager(t *testing.T) {
	sm := NewSecretManager()

	assert.NotNil(t, sm)
	assert.Empty(t, sm.Names())
}

func TestSecretManager_Load_FromEnv(t *testing.T) {
	t.Setenv("TEST_SECRET", "secret_value")

	sm := NewSecretManager()
	err := sm.Load([]SecretDeclaration{
		{Name: "my_secret", From: SecretSource{Env: "TEST_SECRET"}},
	})

	require.NoError(t, err)
	value, ok := sm.Get("my_secret")
	assert.True(t, ok)
	assert.Equal(t, "secret_value", value)
}

func TestSecretManager_Load_FromFile(t *testing.T) {
	tempDir := t.TempDir()
	secretFile := filepath.Join(tempDir, "secret")
	require.NoError(t, os.WriteFile(secretFile, []byte("file_secret_value\n"), fileMode))

	sm := NewSecretManager()
	err := sm.Load([]SecretDeclaration{
		{Name: "my_secret", From: SecretSource{File: secretFile}},
	})

	require.NoError(t, err)
	value, ok := sm.Get("my_secret")
	assert.True(t, ok)
	assert.Equal(t, "file_secret_value", value) // trimmed
}

func TestSecretManager_Load_EnvFallbackToFile(t *testing.T) {
	// Env not set, should fall back to file
	tempDir := t.TempDir()
	secretFile := filepath.Join(tempDir, "secret")
	require.NoError(t, os.WriteFile(secretFile, []byte("fallback_value"), fileMode))

	sm := NewSecretManager()
	err := sm.Load([]SecretDeclaration{
		{Name: "my_secret", From: SecretSource{Env: "NONEXISTENT_VAR", File: secretFile}},
	})

	require.NoError(t, err)
	value, ok := sm.Get("my_secret")
	assert.True(t, ok)
	assert.Equal(t, "fallback_value", value)
}

func TestSecretManager_Load_EnvPreferredOverFile(t *testing.T) {
	t.Setenv("TEST_SECRET", "env_value")

	tempDir := t.TempDir()
	secretFile := filepath.Join(tempDir, "secret")
	require.NoError(t, os.WriteFile(secretFile, []byte("file_value"), fileMode))

	sm := NewSecretManager()
	err := sm.Load([]SecretDeclaration{
		{Name: "my_secret", From: SecretSource{Env: "TEST_SECRET", File: secretFile}},
	})

	require.NoError(t, err)
	value, ok := sm.Get("my_secret")
	assert.True(t, ok)
	assert.Equal(t, "env_value", value) // env takes precedence
}

func TestSecretManager_Load_MissingEnv(t *testing.T) {
	sm := NewSecretManager()
	err := sm.Load([]SecretDeclaration{
		{Name: "my_secret", From: SecretSource{Env: "NONEXISTENT_VAR"}},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSecretEnvNotSet)
}

func TestSecretManager_Load_MissingFile(t *testing.T) {
	sm := NewSecretManager()
	err := sm.Load([]SecretDeclaration{
		{Name: "my_secret", From: SecretSource{File: "/nonexistent/path"}},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSecretFileRead)
}

func TestSecretManager_Load_EmptySource(t *testing.T) {
	sm := NewSecretManager()
	err := sm.Load([]SecretDeclaration{
		{Name: "my_secret", From: SecretSource{}},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSecretSourceEmpty)
}

func TestSecretManager_Get_NotFound(t *testing.T) {
	sm := NewSecretManager()

	_, ok := sm.Get("nonexistent")
	assert.False(t, ok)
}

func TestSecretManager_Has(t *testing.T) {
	sm := NewSecretManager()
	sm.Set("existing", "value")

	assert.True(t, sm.Has("existing"))
	assert.False(t, sm.Has("nonexistent"))
}

func TestSecretManager_Names(t *testing.T) {
	sm := NewSecretManager()
	sm.Set("secret1", "value1")
	sm.Set("secret2", "value2")

	names := sm.Names()
	assert.Len(t, names, 2)
	assert.Contains(t, names, "secret1")
	assert.Contains(t, names, "secret2")
}

func TestSecretManager_Mask(t *testing.T) {
	sm := NewSecretManager()
	sm.Set("api_key", "sk-12345")
	sm.Set("password", "secret123")

	input := "Using API key sk-12345 with password secret123"
	masked := sm.Mask(input)

	assert.Equal(t, "Using API key *** with password ***", masked)
	assert.NotContains(t, masked, "sk-12345")
	assert.NotContains(t, masked, "secret123")
}

func TestSecretManager_Mask_EmptySecret(t *testing.T) {
	sm := NewSecretManager()
	sm.Set("empty", "")

	input := "This should not change"
	masked := sm.Mask(input)

	assert.Equal(t, input, masked)
}

func TestSecretManager_Clone(t *testing.T) {
	sm := NewSecretManager()
	sm.Set("original", "value")

	clone := sm.Clone()
	clone.Set("new", "new_value")

	// Original should not have the new secret
	assert.False(t, sm.Has("new"))
	// Clone should have both
	assert.True(t, clone.Has("original"))
	assert.True(t, clone.Has("new"))
}

func TestSecretManager_Clone_Override(t *testing.T) {
	sm := NewSecretManager()
	sm.Set("api_key", "original_value")

	clone := sm.Clone()
	clone.Set("api_key", "overridden_value")

	// Original should keep its value
	value, _ := sm.Get("api_key")
	assert.Equal(t, "original_value", value)

	// Clone should have the overridden value
	value, _ = clone.Get("api_key")
	assert.Equal(t, "overridden_value", value)
}

func TestSecretManager_Clear(t *testing.T) {
	sm := NewSecretManager()
	sm.Set("secret", "value")

	sm.Clear()

	assert.False(t, sm.Has("secret"))
}
