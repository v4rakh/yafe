package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRoles(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      []Role
		wantErr   bool
		errSubstr string
	}{
		{
			name:  "empty string",
			input: "",
			want:  nil,
		},
		{
			name:  "single role",
			input: "jobs:read",
			want:  []Role{RoleJobsread},
		},
		{
			name:  "multiple roles",
			input: "jobs:read,jobs:write,flows:read",
			want:  []Role{RoleJobsread, RoleJobswrite, RoleFlowsread},
		},
		{
			name:  "all roles",
			input: "jobs:read,jobs:write,flows:read,flows:write",
			want:  []Role{RoleJobsread, RoleJobswrite, RoleFlowsread, RoleFlowswrite},
		},
		{
			name:  "with whitespace",
			input: " jobs:read , jobs:write ",
			want:  []Role{RoleJobsread, RoleJobswrite},
		},
		{
			name:  "deduplicates roles",
			input: "jobs:read,jobs:read,jobs:read",
			want:  []Role{RoleJobsread},
		},
		{
			name:      "invalid role",
			input:     "invalid:role",
			wantErr:   true,
			errSubstr: "not a valid Role",
		},
		{
			name:      "partial invalid",
			input:     "jobs:read,invalid",
			wantErr:   true,
			errSubstr: "not a valid Role",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRoles(tt.input)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestUser_HasRole(t *testing.T) {
	t.Run("returns true when user has role", func(t *testing.T) {
		user := &User{
			Name:  "test",
			Roles: []Role{RoleJobsread, RoleJobswrite},
		}

		assert.True(t, user.HasRole(RoleJobsread))
		assert.True(t, user.HasRole(RoleJobswrite))
	})

	t.Run("returns false when user lacks role", func(t *testing.T) {
		user := &User{
			Name:  "test",
			Roles: []Role{RoleJobsread},
		}

		assert.False(t, user.HasRole(RoleJobswrite))
		assert.False(t, user.HasRole(RoleFlowsread))
	})

	t.Run("returns false for empty roles", func(t *testing.T) {
		user := &User{
			Name:  "test",
			Roles: nil,
		}

		assert.False(t, user.HasRole(RoleJobsread))
	})
}

func TestUser_ValidateKey(t *testing.T) {
	t.Run("returns true for valid key", func(t *testing.T) {
		key := "my-secret-api-key"
		hash, err := HashKey(key)
		require.NoError(t, err)

		user := &User{
			Name: "test",
			Hash: hash,
		}

		assert.True(t, user.ValidateKey(key))
	})

	t.Run("returns false for invalid key", func(t *testing.T) {
		hash, err := HashKey("correct-key")
		require.NoError(t, err)

		user := &User{
			Name: "test",
			Hash: hash,
		}

		assert.False(t, user.ValidateKey("wrong-key"))
	})

	t.Run("returns false for empty key", func(t *testing.T) {
		hash, err := HashKey("some-key")
		require.NoError(t, err)

		user := &User{
			Name: "test",
			Hash: hash,
		}

		assert.False(t, user.ValidateKey(""))
	})
}

func TestHashKey(t *testing.T) {
	t.Run("generates valid bcrypt hash", func(t *testing.T) {
		hash, err := HashKey("test-key")

		require.NoError(t, err)
		assert.NotEmpty(t, hash)
		// Bcrypt hashes start with $2
		assert.True(t, hash[0] == '$' && hash[1] == '2')
	})

	t.Run("different keys produce different hashes", func(t *testing.T) {
		hash1, err := HashKey("key1")
		require.NoError(t, err)

		hash2, err := HashKey("key2")
		require.NoError(t, err)

		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("same key produces different hashes due to salt", func(t *testing.T) {
		hash1, err := HashKey("same-key")
		require.NoError(t, err)

		hash2, err := HashKey("same-key")
		require.NoError(t, err)

		// Different salts mean different hashes
		assert.NotEqual(t, hash1, hash2)
	})
}

func TestUserContext(t *testing.T) {
	t.Run("WithUser and GetUserFromContext roundtrip", func(t *testing.T) {
		user := &User{
			Name:  "testuser",
			Roles: []Role{RoleJobsread},
		}

		ctx := WithUser(context.Background(), user)
		retrieved := GetUserFromContext(ctx)

		require.NotNil(t, retrieved)
		assert.Equal(t, user.Name, retrieved.Name)
		assert.Equal(t, user.Roles, retrieved.Roles)
	})

	t.Run("GetUserFromContext returns nil for empty context", func(t *testing.T) {
		retrieved := GetUserFromContext(context.Background())

		assert.Nil(t, retrieved)
	})

	t.Run("GetUserFromContext returns nil for wrong type in context", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), userContextKey, "not a user")
		retrieved := GetUserFromContext(ctx)

		assert.Nil(t, retrieved)
	})
}
