package scheduler

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"git.myservermanager.com/varakh/yafe/internal/queue"
	"git.myservermanager.com/varakh/yafe/internal/registry"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
)

// Service defines the interface for schedule management.
type Service interface {
	Create(schedule *Schedule) error
	Update(schedule *Schedule) error
	Delete(name string) error
	Enable(name string) error
	Disable(name string) error
	List() ([]*ScheduleWithNextRun, error)
	Get(name string) (*ScheduleWithNextRun, error)
	Exists(name string) bool
	UpdateFlowReferences(oldName, newName string) error
}

// Scheduler manages scheduled job triggers.
type Scheduler struct {
	store    Store
	queue    queue.Queue
	registry registry.FlowRegistry
	cron     *cron.Cron
	mu       sync.RWMutex
	entries  map[string]cron.EntryID // schedule name -> cron entry
}

// New creates a new scheduler.
func New(store Store, q queue.Queue, reg registry.FlowRegistry) *Scheduler {
	return &Scheduler{
		store:    store,
		queue:    q,
		registry: reg,
		cron:     cron.New(cron.WithSeconds()),
		entries:  make(map[string]cron.EntryID),
	}
}

// Start loads all schedules and starts enabled ones.
func (s *Scheduler) Start(_ context.Context) error {
	schedules, err := s.store.List()
	if err != nil {
		return fmt.Errorf("loading schedules: %w", err)
	}

	for _, schedule := range schedules {
		if schedule.Enabled {
			if err := s.addEntry(schedule); err != nil {
				log.Warn().
					Err(err).
					Str("schedule", schedule.Name).
					Msg("Failed to start schedule")
			}
		}
	}

	s.cron.Start()
	log.Info().Int("count", len(schedules)).Msg("Scheduler started")

	return nil
}

// Stop gracefully shuts down the scheduler.
func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
	log.Info().Msg("Scheduler stopped")
}

// Create adds a new schedule.
func (s *Scheduler) Create(schedule *Schedule) error {
	if err := ValidateScheduleName(schedule.Name); err != nil {
		return err
	}
	if err := ValidateScheduleType(schedule.Type); err != nil {
		return err
	}
	if err := s.validateExpression(schedule); err != nil {
		return err
	}
	if !s.registry.Exists(schedule.Flow) {
		return fmt.Errorf("%w: %s", ErrFlowNotFound, schedule.Flow)
	}

	if s.store.Exists(schedule.Name) {
		return fmt.Errorf("%w: %s", ErrScheduleExists, schedule.Name)
	}

	schedule.CreatedAt = time.Now().UTC()

	if err := s.store.Save(schedule); err != nil {
		return err
	}

	if schedule.Enabled {
		if err := s.addEntry(schedule); err != nil {
			return err
		}
	}

	log.Info().
		Str("schedule", schedule.Name).
		Str("flow", schedule.Flow).
		Str("type", string(schedule.Type)).
		Bool("enabled", schedule.Enabled).
		Msg("Schedule created")

	return nil
}

// Update modifies an existing schedule.
func (s *Scheduler) Update(schedule *Schedule) error {
	if err := ValidateScheduleName(schedule.Name); err != nil {
		return err
	}
	if err := ValidateScheduleType(schedule.Type); err != nil {
		return err
	}
	if err := s.validateExpression(schedule); err != nil {
		return err
	}
	if !s.registry.Exists(schedule.Flow) {
		return fmt.Errorf("%w: %s", ErrFlowNotFound, schedule.Flow)
	}

	existing, err := s.store.Get(schedule.Name)
	if err != nil {
		return err
	}

	// Preserve creation time and last run
	schedule.CreatedAt = existing.CreatedAt
	schedule.LastRunAt = existing.LastRunAt

	// Stop old entry if running
	s.removeEntry(schedule.Name)

	if err := s.store.Save(schedule); err != nil {
		return err
	}

	if schedule.Enabled {
		if err := s.addEntry(schedule); err != nil {
			return err
		}
	}

	log.Info().
		Str("schedule", schedule.Name).
		Msg("Schedule updated")

	return nil
}

// Delete removes a schedule.
func (s *Scheduler) Delete(name string) error {
	if err := ValidateScheduleName(name); err != nil {
		return err
	}

	s.removeEntry(name)

	if err := s.store.Delete(name); err != nil {
		return err
	}

	log.Info().Str("schedule", name).Msg("Schedule deleted")
	return nil
}

// Enable starts a disabled schedule.
func (s *Scheduler) Enable(name string) error {
	schedule, err := s.store.Get(name)
	if err != nil {
		return err
	}

	if schedule.Enabled {
		return nil // Already enabled
	}

	schedule.Enabled = true
	if err := s.store.Save(schedule); err != nil {
		return err
	}

	if err := s.addEntry(schedule); err != nil {
		return err
	}

	log.Info().Str("schedule", name).Msg("Schedule enabled")
	return nil
}

// Disable stops an enabled schedule.
func (s *Scheduler) Disable(name string) error {
	schedule, err := s.store.Get(name)
	if err != nil {
		return err
	}

	if !schedule.Enabled {
		return nil // Already disabled
	}

	s.removeEntry(name)

	schedule.Enabled = false
	if err := s.store.Save(schedule); err != nil {
		return err
	}

	log.Info().Str("schedule", name).Msg("Schedule disabled")
	return nil
}

// List returns all schedules with computed next run times.
func (s *Scheduler) List() ([]*ScheduleWithNextRun, error) {
	schedules, err := s.store.List()
	if err != nil {
		return nil, err
	}

	result := make([]*ScheduleWithNextRun, 0, len(schedules))
	for _, schedule := range schedules {
		result = append(result, s.withNextRun(schedule))
	}

	return result, nil
}

// Get retrieves a schedule with computed next run time.
func (s *Scheduler) Get(name string) (*ScheduleWithNextRun, error) {
	schedule, err := s.store.Get(name)
	if err != nil {
		return nil, err
	}

	return s.withNextRun(schedule), nil
}

// withNextRun computes the next run time for a schedule.
func (s *Scheduler) withNextRun(schedule *Schedule) *ScheduleWithNextRun {
	result := &ScheduleWithNextRun{Schedule: schedule}

	s.mu.RLock()
	entryID, ok := s.entries[schedule.Name]
	if ok {
		entry := s.cron.Entry(entryID)
		if !entry.Next.IsZero() {
			next := entry.Next
			result.NextRunAt = &next
		}
	}
	s.mu.RUnlock()

	return result
}

// addEntry adds a cron entry for a schedule.
func (s *Scheduler) addEntry(schedule *Schedule) error {
	spec := s.cronSpec(schedule)

	entryID, err := s.cron.AddFunc(spec, func() {
		s.scheduleJob(schedule)
	})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidExpression, err)
	}

	s.mu.Lock()
	s.entries[schedule.Name] = entryID
	s.mu.Unlock()

	return nil
}

// removeEntry removes a cron entry for a schedule.
func (s *Scheduler) removeEntry(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entryID, ok := s.entries[name]; ok {
		s.cron.Remove(entryID)
		delete(s.entries, name)
	}
}

// scheduleJob is called by cron to enqueue a job.
func (s *Scheduler) scheduleJob(schedule *Schedule) {
	// Reload schedule to get latest inputs
	current, err := s.store.Get(schedule.Name)
	if err != nil {
		log.Error().
			Err(err).
			Str("schedule", schedule.Name).
			Msg("Failed to load schedule for job")
		return
	}

	job, err := s.queue.Enqueue(current.Flow, current.Inputs)
	if err != nil {
		log.Error().
			Err(err).
			Str("schedule", schedule.Name).
			Str("flow", current.Flow).
			Msg("Failed to enqueue scheduled job")
		return
	}

	// Update last run time
	now := time.Now().UTC()
	current.LastRunAt = &now
	if err := s.store.Save(current); err != nil {
		log.Warn().
			Err(err).
			Str("schedule", schedule.Name).
			Msg("Failed to update last run time")
	}

	log.Info().
		Str("schedule", schedule.Name).
		Str("job_id", job.ID).
		Str("flow", current.Flow).
		Msg("Scheduled job enqueued")
}

// cronSpec returns the cron specification for a schedule.
func (s *Scheduler) cronSpec(schedule *Schedule) string {
	switch schedule.Type {
	case ScheduleTypeInterval:
		return "@every " + schedule.Expression
	case ScheduleTypeCron:
		return schedule.Expression
	default:
		return schedule.Expression
	}
}

// validateExpression validates the schedule expression.
func (s *Scheduler) validateExpression(schedule *Schedule) error {
	spec := s.cronSpec(schedule)
	parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	_, err := parser.Parse(spec)
	if err != nil {
		if schedule.Type == ScheduleTypeInterval {
			return fmt.Errorf("%w: invalid interval %q: %v", ErrInvalidExpression, schedule.Expression, err)
		}
		return fmt.Errorf("%w: invalid cron expression %q: %v", ErrInvalidExpression, schedule.Expression, err)
	}
	return nil
}

// Exists checks if a schedule exists.
func (s *Scheduler) Exists(name string) bool {
	name = strings.TrimSpace(name)
	return s.store.Exists(name)
}

// UpdateFlowReferences updates all schedules that reference oldName to use newName.
// Uses best-effort approach: logs errors but continues processing.
func (s *Scheduler) UpdateFlowReferences(oldName, newName string) error {
	schedules, err := s.store.List()
	if err != nil {
		return fmt.Errorf("listing schedules: %w", err)
	}

	for _, schedule := range schedules {
		if schedule.Flow == oldName {
			schedule.Flow = newName
			if err := s.store.Save(schedule); err != nil {
				log.Warn().
					Err(err).
					Str("schedule", schedule.Name).
					Str("old_flow", oldName).
					Str("new_flow", newName).
					Msg("Failed to update schedule flow reference")
				continue
			}
			log.Info().
				Str("schedule", schedule.Name).
				Str("old_flow", oldName).
				Str("new_flow", newName).
				Msg("Updated schedule flow reference")
		}
	}

	return nil
}
