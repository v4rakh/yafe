package registry

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRegistry(t *testing.T) *FileFlowRegistry {
	t.Helper()
	reg, err := NewFileFlowRegistry(t.TempDir())
	require.NoError(t, err)
	return reg
}

func TestNewFileFlowRegistry(t *testing.T) {
	t.Run("creates directory if not exists", func(t *testing.T) {
		tmpDir := filepath.Join(t.TempDir(), "flows")
		reg, err := NewFileFlowRegistry(tmpDir)

		require.NoError(t, err)
		assert.NotNil(t, reg)
		assert.DirExists(t, tmpDir)
	})

	t.Run("returns directory path", func(t *testing.T) {
		tmpDir := t.TempDir()
		reg, err := NewFileFlowRegistry(tmpDir)

		require.NoError(t, err)
		assert.Equal(t, tmpDir, reg.Dir())
	})
}

func TestFileFlowRegistry_Add(t *testing.T) {
	t.Run("creates flow file", func(t *testing.T) {
		reg := setupTestRegistry(t)
		content := []byte("runs-on: host\nsteps:\n  - cmd: echo test\n    kind: shell")

		err := reg.Add("test-flow", content)

		require.NoError(t, err)
		assert.FileExists(t, reg.Path("test-flow"))

		// Verify content
		data, err := os.ReadFile(reg.Path("test-flow"))
		require.NoError(t, err)
		assert.Equal(t, content, data)
	})

	t.Run("overwrites existing flow", func(t *testing.T) {
		reg := setupTestRegistry(t)
		original := []byte("original content")
		updated := []byte("updated content")

		require.NoError(t, reg.Add("myflow", original))
		require.NoError(t, reg.Add("myflow", updated))

		data, err := os.ReadFile(reg.Path("myflow"))
		require.NoError(t, err)
		assert.Equal(t, updated, data)
	})

	t.Run("rejects invalid flow name", func(t *testing.T) {
		reg := setupTestRegistry(t)
		content := []byte("test")

		tests := []struct {
			name    string
			flow    string
			wantErr string
		}{
			{"empty", "", "empty flow name"},
			{"whitespace only", "   ", "empty flow name"},
			{"contains spaces", "my flow", "alphanumeric"},
			{"contains slash", "my/flow", "alphanumeric"},
			{"contains dot", "my.flow", "alphanumeric"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := reg.Add(tt.flow, content)
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidFlowName)
				assert.Contains(t, err.Error(), tt.wantErr)
			})
		}
	})

	t.Run("accepts valid flow names", func(t *testing.T) {
		reg := setupTestRegistry(t)
		content := []byte("test")

		validNames := []string{
			"myflow",
			"my-flow",
			"my_flow",
			"MyFlow123",
			"FLOW",
			"a",
		}

		for _, name := range validNames {
			t.Run(name, func(t *testing.T) {
				err := reg.Add(name, content)
				require.NoError(t, err)
			})
		}
	})
}

func TestFileFlowRegistry_Get(t *testing.T) {
	t.Run("returns flow content", func(t *testing.T) {
		reg := setupTestRegistry(t)
		expected := []byte("flow content here")
		require.NoError(t, reg.Add("test", expected))

		content, err := reg.Get("test")

		require.NoError(t, err)
		assert.Equal(t, expected, content)
	})

	t.Run("trims whitespace from name", func(t *testing.T) {
		reg := setupTestRegistry(t)
		expected := []byte("content")
		require.NoError(t, reg.Add("myflow", expected))

		content, err := reg.Get("  myflow  ")

		require.NoError(t, err)
		assert.Equal(t, expected, content)
	})

	t.Run("returns error for nonexistent flow", func(t *testing.T) {
		reg := setupTestRegistry(t)

		_, err := reg.Get("nonexistent")

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrFlowNotFound)
	})

	t.Run("returns error for invalid name", func(t *testing.T) {
		reg := setupTestRegistry(t)

		_, err := reg.Get("invalid/name")

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidFlowName)
	})
}

func TestFileFlowRegistry_List(t *testing.T) {
	t.Run("returns empty list when no flows", func(t *testing.T) {
		reg := setupTestRegistry(t)

		flows, err := reg.List()

		require.NoError(t, err)
		assert.Empty(t, flows)
	})

	t.Run("returns sorted flow names", func(t *testing.T) {
		reg := setupTestRegistry(t)
		require.NoError(t, reg.Add("zebra", []byte("test")))
		require.NoError(t, reg.Add("alpha", []byte("test")))
		require.NoError(t, reg.Add("middle", []byte("test")))

		flows, err := reg.List()

		require.NoError(t, err)
		assert.Equal(t, []string{"alpha", "middle", "zebra"}, flows)
	})

	t.Run("ignores non-yaml files", func(t *testing.T) {
		reg := setupTestRegistry(t)
		require.NoError(t, reg.Add("valid", []byte("test")))
		// Create a non-yaml file
		require.NoError(t, os.WriteFile(filepath.Join(reg.Dir(), "notayaml.txt"), []byte("test"), 0600))

		flows, err := reg.List()

		require.NoError(t, err)
		assert.Equal(t, []string{"valid"}, flows)
	})

	t.Run("handles both yaml and yml extensions", func(t *testing.T) {
		reg := setupTestRegistry(t)
		require.NoError(t, reg.Add("flow1", []byte("test")))
		// Create a .yml file manually
		require.NoError(t, os.WriteFile(filepath.Join(reg.Dir(), "flow2.yml"), []byte("test"), 0600))

		flows, err := reg.List()

		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"flow1", "flow2"}, flows)
	})
}

func TestFileFlowRegistry_Delete(t *testing.T) {
	t.Run("removes flow file", func(t *testing.T) {
		reg := setupTestRegistry(t)
		require.NoError(t, reg.Add("todelete", []byte("test")))
		assert.FileExists(t, reg.Path("todelete"))

		err := reg.Delete("todelete")

		require.NoError(t, err)
		assert.NoFileExists(t, reg.Path("todelete"))
	})

	t.Run("returns error for nonexistent flow", func(t *testing.T) {
		reg := setupTestRegistry(t)

		err := reg.Delete("nonexistent")

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrFlowNotFound)
	})

	t.Run("returns error for invalid name", func(t *testing.T) {
		reg := setupTestRegistry(t)

		err := reg.Delete("invalid/name")

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidFlowName)
	})
}

func TestFileFlowRegistry_Exists(t *testing.T) {
	t.Run("returns true for existing flow", func(t *testing.T) {
		reg := setupTestRegistry(t)
		require.NoError(t, reg.Add("exists", []byte("test")))

		assert.True(t, reg.Exists("exists"))
	})

	t.Run("returns false for nonexistent flow", func(t *testing.T) {
		reg := setupTestRegistry(t)

		assert.False(t, reg.Exists("nonexistent"))
	})
}

func TestFileFlowRegistry_Path(t *testing.T) {
	t.Run("returns correct path with yaml extension", func(t *testing.T) {
		reg := setupTestRegistry(t)
		expected := filepath.Join(reg.Dir(), "myflow.yaml")

		assert.Equal(t, expected, reg.Path("myflow"))
	})
}

func TestValidateFlowName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "myflow", false},
		{"valid with hyphen", "my-flow", false},
		{"valid with underscore", "my_flow", false},
		{"valid with numbers", "flow123", false},
		{"valid mixed case", "MyFlow", false},
		{"invalid empty", "", true},
		{"invalid whitespace", "   ", true},
		{"invalid with spaces", "my flow", true},
		{"invalid with slash", "my/flow", true},
		{"invalid with dot", "my.flow", true},
		{"invalid with colon", "my:flow", true},
		{"invalid path traversal", "../etc/passwd", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFlowName(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidFlowName)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
