package scheduler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
)

const scheduleFileExtension = ".json"

// Store defines the interface for schedule storage.
type Store interface {
	List() ([]*Schedule, error)
	Get(name string) (*Schedule, error)
	Save(schedule *Schedule) error
	Delete(name string) error
	Exists(name string) bool
}

// FileStore is a file-based implementation of Store.
type FileStore struct {
	dir string
	mu  sync.RWMutex
}

// NewFileStore creates a new file-based schedule store.
func NewFileStore(dir string) (*FileStore, error) {
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("creating schedules directory: %w", err)
	}
	return &FileStore{dir: dir}, nil
}

// Dir returns the schedules directory path.
func (s *FileStore) Dir() string {
	return s.dir
}

// path returns the full path to a schedule file.
func (s *FileStore) path(name string) string {
	return filepath.Join(s.dir, name+scheduleFileExtension)
}

// Exists checks if a schedule exists.
func (s *FileStore) Exists(name string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, err := os.Stat(s.path(name))
	return err == nil
}

// List returns all schedules.
func (s *FileStore) List() ([]*Schedule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*Schedule{}, nil
		}
		return nil, fmt.Errorf("reading schedules directory: %w", err)
	}

	var schedules []*Schedule
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, scheduleFileExtension) {
			continue
		}

		scheduleName := strings.TrimSuffix(name, scheduleFileExtension)
		schedule, err := s.readSchedule(scheduleName)
		if err != nil {
			continue // Skip malformed schedules
		}
		schedules = append(schedules, schedule)
	}

	// Sort by name
	sort.Slice(schedules, func(i, j int) bool {
		return schedules[i].Name < schedules[j].Name
	})

	return schedules, nil
}

// Get retrieves a schedule by name.
func (s *FileStore) Get(name string) (*Schedule, error) {
	name = strings.TrimSpace(name)
	if err := ValidateScheduleName(name); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.readSchedule(name)
}

// Save creates or updates a schedule.
func (s *FileStore) Save(schedule *Schedule) error {
	name := strings.TrimSpace(schedule.Name)
	if err := ValidateScheduleName(name); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.writeSchedule(schedule)
}

// Delete removes a schedule.
func (s *FileStore) Delete(name string) error {
	name = strings.TrimSpace(name)
	if err := ValidateScheduleName(name); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	schedulePath := s.path(name)
	if _, err := os.Stat(schedulePath); os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", ErrScheduleNotFound, name)
	}

	if err := os.Remove(schedulePath); err != nil {
		return fmt.Errorf("deleting schedule file: %w", err)
	}

	return nil
}

// readSchedule reads a schedule from disk (caller must hold lock).
func (s *FileStore) readSchedule(name string) (*Schedule, error) {
	schedulePath := s.path(name)
	//gosec:disable G304 -- Path is constructed from trusted base directory
	data, err := os.ReadFile(schedulePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrScheduleNotFound, name)
		}
		return nil, fmt.Errorf("reading schedule file: %w", err)
	}

	var schedule Schedule
	if err := json.Unmarshal(data, &schedule); err != nil {
		return nil, fmt.Errorf("unmarshaling schedule: %w", err)
	}

	return &schedule, nil
}

// writeSchedule writes a schedule to disk (caller must hold lock).
func (s *FileStore) writeSchedule(schedule *Schedule) error {
	data, err := json.MarshalIndent(schedule, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling schedule: %w", err)
	}

	schedulePath := s.path(schedule.Name)
	tmpPath := schedulePath + ".tmp"

	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("writing schedule file: %w", err)
	}

	if err := os.Rename(tmpPath, schedulePath); err != nil {
		if removeErr := os.Remove(tmpPath); removeErr != nil && !os.IsNotExist(removeErr) {
			log.Warn().Err(removeErr).Str("path", tmpPath).Msg("Failed to clean up temp file")
		}
		return fmt.Errorf("renaming schedule file: %w", err)
	}

	return nil
}
