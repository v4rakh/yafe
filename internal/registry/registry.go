package registry

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/rs/zerolog/log"
)

const (
	flowFileExtension    = ".yaml"
	flowFileExtensionAlt = ".yml"
)

var (
	ErrFlowNotFound    = errors.New("flow not found")
	ErrInvalidFlowName = errors.New("invalid flow name")
	ErrFlowExists      = errors.New("flow already exists")

	flowNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
)

// FlowRegistry manages flow definitions.
type FlowRegistry interface {
	List() ([]string, error)
	Get(name string) ([]byte, error)
	Add(name string, content []byte) error
	Delete(name string) error
	Rename(oldName, newName string) error
	Path(name string) string
	Dir() string
	Exists(name string) bool
}

// FileFlowRegistry is a file-based implementation of FlowRegistry.
type FileFlowRegistry struct {
	dir string
}

// NewFileFlowRegistry creates a new file-based flow registry.
func NewFileFlowRegistry(dir string) (*FileFlowRegistry, error) {
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("creating flows directory: %w", err)
	}
	return &FileFlowRegistry{dir: dir}, nil
}

// Dir returns the flows directory path.
func (r *FileFlowRegistry) Dir() string {
	return r.dir
}

// Path returns the full path to a flow file.
func (r *FileFlowRegistry) Path(name string) string {
	return filepath.Join(r.dir, name+flowFileExtension)
}

// Exists checks if a flow exists.
func (r *FileFlowRegistry) Exists(name string) bool {
	_, err := os.Stat(r.Path(name))
	return err == nil
}

// List returns a list of all flow names.
func (r *FileFlowRegistry) List() ([]string, error) {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("reading flows directory: %w", err)
	}

	var flows []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, flowFileExtension) || strings.HasSuffix(name, flowFileExtensionAlt) {
			flowName := strings.TrimSuffix(strings.TrimSuffix(name, flowFileExtension), flowFileExtensionAlt)
			flows = append(flows, flowName)
		}
	}

	sort.Strings(flows)
	return flows, nil
}

// Get returns the content of a flow file.
func (r *FileFlowRegistry) Get(name string) ([]byte, error) {
	name = strings.TrimSpace(name)
	if err := ValidateFlowName(name); err != nil {
		return nil, err
	}

	flowPath := r.Path(name)
	//gosec:disable G304 -- Explicitly allowed to use base location
	data, err := os.ReadFile(flowPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrFlowNotFound, name)
		}
		return nil, fmt.Errorf("reading flow file: %w", err)
	}

	return data, nil
}

// Delete removes a flow file.
func (r *FileFlowRegistry) Delete(name string) error {
	name = strings.TrimSpace(name)
	if err := ValidateFlowName(name); err != nil {
		return err
	}

	flowPath := r.Path(name)
	if _, err := os.Stat(flowPath); os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", ErrFlowNotFound, name)
	}

	if err := os.Remove(flowPath); err != nil {
		return fmt.Errorf("deleting flow file: %w", err)
	}

	return nil
}

// Add creates or replaces a flow file.
func (r *FileFlowRegistry) Add(name string, content []byte) error {
	name = strings.TrimSpace(name)
	if err := ValidateFlowName(name); err != nil {
		return err
	}

	flowPath := r.Path(name)
	tmpPath := flowPath + ".tmp"

	if err := os.WriteFile(tmpPath, content, 0600); err != nil {
		return fmt.Errorf("writing flow file: %w", err)
	}

	if err := os.Rename(tmpPath, flowPath); err != nil {
		if removeErr := os.Remove(tmpPath); removeErr != nil && !os.IsNotExist(removeErr) {
			log.Warn().Err(removeErr).Str("path", tmpPath).Msg("Failed to clean up temp file")
		}
		return fmt.Errorf("renaming flow file: %w", err)
	}

	return nil
}

// Rename renames a flow from oldName to newName.
func (r *FileFlowRegistry) Rename(oldName, newName string) error {
	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)

	// Validate both names
	if err := ValidateFlowName(oldName); err != nil {
		return err
	}
	if err := ValidateFlowName(newName); err != nil {
		return err
	}

	// Check old flow exists
	oldPath := r.Path(oldName)
	if _, err := os.Stat(oldPath); os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", ErrFlowNotFound, oldName)
	}

	// Check new name doesn't exist
	newPath := r.Path(newName)
	if _, err := os.Stat(newPath); err == nil {
		return fmt.Errorf("%w: %s", ErrFlowExists, newName)
	}

	// Read old file content
	//gosec:disable G304 -- Explicitly allowed within registry
	content, err := os.ReadFile(oldPath)
	if err != nil {
		return fmt.Errorf("reading flow file: %w", err)
	}

	// Write to new file using atomic pattern
	tmpPath := newPath + ".tmp"
	if err := os.WriteFile(tmpPath, content, 0600); err != nil { //nolint:gosec
		return fmt.Errorf("writing flow file: %w", err)
	}

	if err := os.Rename(tmpPath, newPath); err != nil {
		if removeErr := os.Remove(tmpPath); removeErr != nil && !os.IsNotExist(removeErr) {
			log.Warn().Err(removeErr).Str("path", tmpPath).Msg("Failed to clean up temp file")
		}
		return fmt.Errorf("renaming flow file: %w", err)
	}

	// Delete old file after new file is committed
	if err := os.Remove(oldPath); err != nil {
		// Log warning but return success (new file exists)
		log.Warn().Err(err).Str("path", oldPath).Msg("Failed to delete old flow file after rename")
	}

	return nil
}

// ValidateFlowName checks if a flow name is valid.
func ValidateFlowName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%w: empty flow name", ErrInvalidFlowName)
	}
	if !flowNamePattern.MatchString(name) {
		return fmt.Errorf("%w: must contain only alphanumeric characters, underscores, and hyphens", ErrInvalidFlowName)
	}
	return nil
}
