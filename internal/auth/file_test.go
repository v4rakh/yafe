package auth

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestAuthFile(t *testing.T, content string) string {
	t.Helper()
	tmpDir := t.TempDir()
	authFile := filepath.Join(tmpDir, "auth")
	require.NoError(t, os.WriteFile(authFile, []byte(content), 0600))
	return authFile
}

func createAuthLine(t *testing.T, user, key, roles string) string {
	t.Helper()
	hash, err := HashKey(key)
	require.NoError(t, err)
	return user + ":" + string(hash) + ":" + roles
}

func TestNewFileAuthenticator(t *testing.T) {
	t.Run("parses valid auth file", func(t *testing.T) {
		line := createAuthLine(t, "admin", "secret-key", "jobs:read,jobs:write")
		authFile := createTestAuthFile(t, line)

		auth, err := NewFileAuthenticator(authFile)

		require.NoError(t, err)
		require.NotNil(t, auth)
		assert.Len(t, auth.users, 1)
		assert.Equal(t, "admin", auth.users[0].Name)
		assert.Equal(t, []Role{RoleJobsread, RoleJobswrite}, auth.users[0].Roles)
	})

	t.Run("parses multiple users", func(t *testing.T) {
		content := createAuthLine(t, "user1", "key1", "jobs:read") + "\n" +
			createAuthLine(t, "user2", "key2", "flows:read")
		authFile := createTestAuthFile(t, content)

		auth, err := NewFileAuthenticator(authFile)

		require.NoError(t, err)
		assert.Len(t, auth.users, 2)
	})

	t.Run("skips empty lines", func(t *testing.T) {
		content := createAuthLine(t, "user1", "key1", "jobs:read") + "\n\n\n" +
			createAuthLine(t, "user2", "key2", "flows:read")
		authFile := createTestAuthFile(t, content)

		auth, err := NewFileAuthenticator(authFile)

		require.NoError(t, err)
		assert.Len(t, auth.users, 2)
	})

	t.Run("skips comment lines", func(t *testing.T) {
		content := "# This is a comment\n" +
			createAuthLine(t, "user1", "key1", "jobs:read") + "\n" +
			"# Another comment\n" +
			createAuthLine(t, "user2", "key2", "flows:read")
		authFile := createTestAuthFile(t, content)

		auth, err := NewFileAuthenticator(authFile)

		require.NoError(t, err)
		assert.Len(t, auth.users, 2)
	})

	t.Run("returns error for nonexistent file", func(t *testing.T) {
		_, err := NewFileAuthenticator("/nonexistent/path")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "opening auth file")
	})

	t.Run("returns error for duplicate users", func(t *testing.T) {
		content := createAuthLine(t, "admin", "key1", "jobs:read") + "\n" +
			createAuthLine(t, "admin", "key2", "flows:read")
		authFile := createTestAuthFile(t, content)

		_, err := NewFileAuthenticator(authFile)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate user")
	})

	t.Run("returns error for malformed line", func(t *testing.T) {
		authFile := createTestAuthFile(t, "invalid-line-without-colon")

		_, err := NewFileAuthenticator(authFile)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid format")
	})

	t.Run("returns error for empty username", func(t *testing.T) {
		authFile := createTestAuthFile(t, ":$2a$10$somehash:jobs:read")

		_, err := NewFileAuthenticator(authFile)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty username")
	})

	t.Run("returns error for empty hash", func(t *testing.T) {
		authFile := createTestAuthFile(t, "user::jobs:read")

		_, err := NewFileAuthenticator(authFile)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty hash")
	})

	t.Run("returns error for non-bcrypt hash", func(t *testing.T) {
		authFile := createTestAuthFile(t, "user:plaintext:jobs:read")

		_, err := NewFileAuthenticator(authFile)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid hash format")
	})

	t.Run("returns error for invalid role", func(t *testing.T) {
		// Create a valid bcrypt hash manually
		hash, _ := HashKey("test")
		content := "user:" + string(hash) + ":invalid:role"
		authFile := createTestAuthFile(t, content)

		_, err := NewFileAuthenticator(authFile)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a valid Role")
	})

	t.Run("allows user without roles", func(t *testing.T) {
		hash, _ := HashKey("test")
		content := "user:" + string(hash)
		authFile := createTestAuthFile(t, content)

		auth, err := NewFileAuthenticator(authFile)

		require.NoError(t, err)
		assert.Len(t, auth.users, 1)
		assert.Empty(t, auth.users[0].Roles)
	})
}

func TestFileAuthenticator_Authenticate(t *testing.T) {
	t.Run("returns user for valid key", func(t *testing.T) {
		line := createAuthLine(t, "admin", "secret-key", "jobs:read")
		authFile := createTestAuthFile(t, line)
		auth, err := NewFileAuthenticator(authFile)
		require.NoError(t, err)

		user := auth.Authenticate("secret-key")

		require.NotNil(t, user)
		assert.Equal(t, "admin", user.Name)
		assert.Equal(t, []Role{RoleJobsread}, user.Roles)
	})

	t.Run("returns nil for invalid key", func(t *testing.T) {
		line := createAuthLine(t, "admin", "correct-key", "jobs:read")
		authFile := createTestAuthFile(t, line)
		auth, err := NewFileAuthenticator(authFile)
		require.NoError(t, err)

		user := auth.Authenticate("wrong-key")

		assert.Nil(t, user)
	})

	t.Run("returns nil for empty key", func(t *testing.T) {
		line := createAuthLine(t, "admin", "secret-key", "jobs:read")
		authFile := createTestAuthFile(t, line)
		auth, err := NewFileAuthenticator(authFile)
		require.NoError(t, err)

		user := auth.Authenticate("")

		assert.Nil(t, user)
	})

	t.Run("authenticates correct user among multiple", func(t *testing.T) {
		content := createAuthLine(t, "user1", "key1", "jobs:read") + "\n" +
			createAuthLine(t, "user2", "key2", "flows:read")
		authFile := createTestAuthFile(t, content)
		auth, err := NewFileAuthenticator(authFile)
		require.NoError(t, err)

		user := auth.Authenticate("key2")

		require.NotNil(t, user)
		assert.Equal(t, "user2", user.Name)
	})
}

func TestNewInlineAuthenticator(t *testing.T) {
	t.Run("creates authenticator with valid inputs", func(t *testing.T) {
		hash, _ := HashKey("test")

		auth, err := NewInlineAuthenticator("admin", string(hash), "jobs:read,jobs:write")

		require.NoError(t, err)
		require.NotNil(t, auth)
	})

	t.Run("returns error for empty username", func(t *testing.T) {
		hash, _ := HashKey("test")

		_, err := NewInlineAuthenticator("", string(hash), "jobs:read")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "username required")
	})

	t.Run("returns error for empty hash", func(t *testing.T) {
		_, err := NewInlineAuthenticator("admin", "", "jobs:read")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "hash required")
	})

	t.Run("returns error for non-bcrypt hash", func(t *testing.T) {
		_, err := NewInlineAuthenticator("admin", "plaintext", "jobs:read")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid hash format")
	})

	t.Run("returns error for invalid role", func(t *testing.T) {
		hash, _ := HashKey("test")

		_, err := NewInlineAuthenticator("admin", string(hash), "invalid:role")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a valid Role")
	})
}

func TestInlineAuthenticator_Authenticate(t *testing.T) {
	t.Run("returns user for valid key", func(t *testing.T) {
		hash, _ := HashKey("my-api-key")
		auth, err := NewInlineAuthenticator("admin", string(hash), "jobs:read")
		require.NoError(t, err)

		user := auth.Authenticate("my-api-key")

		require.NotNil(t, user)
		assert.Equal(t, "admin", user.Name)
	})

	t.Run("returns nil for invalid key", func(t *testing.T) {
		hash, _ := HashKey("correct-key")
		auth, err := NewInlineAuthenticator("admin", string(hash), "jobs:read")
		require.NoError(t, err)

		user := auth.Authenticate("wrong-key")

		assert.Nil(t, user)
	})
}

func TestMultiAuthenticator(t *testing.T) {
	t.Run("tries authenticators in order", func(t *testing.T) {
		hash1, _ := HashKey("key1")
		hash2, _ := HashKey("key2")

		auth1, _ := NewInlineAuthenticator("user1", string(hash1), "jobs:read")
		auth2, _ := NewInlineAuthenticator("user2", string(hash2), "flows:read")

		multi := NewMultiAuthenticator(auth1, auth2)

		// Authenticate with second user's key
		user := multi.Authenticate("key2")

		require.NotNil(t, user)
		assert.Equal(t, "user2", user.Name)
	})

	t.Run("returns first matching user", func(t *testing.T) {
		hash, _ := HashKey("shared-key")

		auth1, _ := NewInlineAuthenticator("user1", string(hash), "jobs:read")
		auth2, _ := NewInlineAuthenticator("user2", string(hash), "flows:read")

		multi := NewMultiAuthenticator(auth1, auth2)

		user := multi.Authenticate("shared-key")

		require.NotNil(t, user)
		assert.Equal(t, "user1", user.Name) // First one wins
	})

	t.Run("returns nil when no authenticator matches", func(t *testing.T) {
		hash, _ := HashKey("key1")
		auth1, _ := NewInlineAuthenticator("user1", string(hash), "jobs:read")

		multi := NewMultiAuthenticator(auth1)

		user := multi.Authenticate("wrong-key")

		assert.Nil(t, user)
	})
}
