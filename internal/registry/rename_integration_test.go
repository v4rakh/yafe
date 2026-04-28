package registry

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFileFlowRegistry_Rename_Integration(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()

	reg, err := NewFileFlowRegistry(tempDir)
	if err != nil {
		t.Fatalf("Error creating registry: %v", err)
	}

	content := []byte(`version: "1"
name: old-flow
steps:
  - name: test
    kind: shell
    cmd: echo "test"
`)

	t.Run("successful rename preserves content", func(t *testing.T) {
		// Add a test flow
		if err := reg.Add("old-flow", content); err != nil {
			t.Fatalf("Error adding flow: %v", err)
		}

		// Verify old flow exists
		if !reg.Exists("old-flow") {
			t.Fatal("old-flow doesn't exist after adding")
		}

		// Rename the flow
		if err := reg.Rename("old-flow", "new-flow"); err != nil {
			t.Fatalf("Error renaming flow: %v", err)
		}

		// Verify old flow no longer exists
		if reg.Exists("old-flow") {
			t.Error("old-flow still exists after rename")
		}

		// Verify new flow exists
		if !reg.Exists("new-flow") {
			t.Error("new-flow doesn't exist after rename")
		}

		// Verify content is preserved
		newContent, err := reg.Get("new-flow")
		if err != nil {
			t.Fatalf("Error getting new flow: %v", err)
		}
		if string(newContent) != string(content) {
			t.Error("Content not preserved after rename")
		}

		// Verify files on disk
		oldPath := filepath.Join(tempDir, "old-flow.yaml")
		newPath := filepath.Join(tempDir, "new-flow.yaml")

		if _, err := os.Stat(oldPath); err == nil {
			t.Error("old-flow.yaml still exists on disk")
		}
		if _, err := os.Stat(newPath); err != nil {
			t.Errorf("new-flow.yaml doesn't exist on disk: %v", err)
		}
	})

	t.Run("error when old flow doesn't exist", func(t *testing.T) {
		err := reg.Rename("nonexistent", "something")
		if err == nil {
			t.Fatal("Should have failed for non-existent flow")
		}
		if !errors.Is(err, ErrFlowNotFound) && err.Error() != "flow not found: nonexistent" {
			t.Errorf("Wrong error: %v", err)
		}
	})

	t.Run("error when new name already exists", func(t *testing.T) {
		// Create two flows
		if err := reg.Add("flow-a", content); err != nil {
			t.Fatalf("Error adding flow-a: %v", err)
		}
		if err := reg.Add("flow-b", content); err != nil {
			t.Fatalf("Error adding flow-b: %v", err)
		}

		// Try to rename flow-a to flow-b
		err := reg.Rename("flow-a", "flow-b")
		if err == nil {
			t.Fatal("Should have failed for existing target name")
		}
		if !errors.Is(err, ErrFlowExists) && err.Error() != "flow already exists: flow-b" {
			t.Errorf("Wrong error: %v", err)
		}
	})

	t.Run("error for invalid names", func(t *testing.T) {
		if err := reg.Add("valid-flow", content); err != nil {
			t.Fatalf("Error adding valid-flow: %v", err)
		}

		// Invalid old name
		err := reg.Rename("invalid/name", "new-name")
		if err == nil || !errors.Is(err, ErrInvalidFlowName) && err.Error() == "" {
			t.Errorf("Should have failed for invalid old name, got: %v", err)
		}

		// Invalid new name
		err = reg.Rename("valid-flow", "invalid/name")
		if err == nil || !errors.Is(err, ErrInvalidFlowName) && err.Error() == "" {
			t.Errorf("Should have failed for invalid new name, got: %v", err)
		}
	})
}
