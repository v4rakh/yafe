package scheduler

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	ErrScheduleNotFound    = errors.New("schedule not found")
	ErrScheduleExists      = errors.New("schedule already exists")
	ErrInvalidScheduleName = errors.New("invalid schedule name")
	ErrInvalidExpression   = errors.New("invalid expression")
	ErrFlowNotFound        = errors.New("flow not found")

	scheduleNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
)

// Schedule represents a scheduled job trigger.
type Schedule struct {
	Name       string            `json:"name"`
	Flow       string            `json:"flow"`
	Inputs     map[string]string `json:"inputs,omitempty"`
	Type       ScheduleType      `json:"type"`
	Expression string            `json:"expression"`
	Enabled    bool              `json:"enabled"`
	CreatedAt  time.Time         `json:"created_at"`
	LastRunAt  *time.Time        `json:"last_run_at,omitempty"`
}

// ScheduleWithNextRun extends Schedule with computed next run time.
type ScheduleWithNextRun struct {
	*Schedule
	NextRunAt *time.Time `json:"next_run_at,omitempty"`
}

// ValidateScheduleName checks if a schedule name is valid.
func ValidateScheduleName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%w: empty schedule name", ErrInvalidScheduleName)
	}
	if !scheduleNamePattern.MatchString(name) {
		return fmt.Errorf("%w: must contain only alphanumeric characters, underscores, and hyphens", ErrInvalidScheduleName)
	}
	return nil
}

// ValidateScheduleType checks if a schedule type is valid.
func ValidateScheduleType(t ScheduleType) error {
	if !t.IsValid() {
		return ErrInvalidScheduleType
	}
	return nil
}
