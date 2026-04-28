package scheduler

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"git.myservermanager.com/varakh/yafe/internal/queue"
	"git.myservermanager.com/varakh/yafe/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestScheduler(t *testing.T) (*Scheduler, *FileStore, *registry.FileFlowRegistry) {
	t.Helper()
	tmpDir := t.TempDir()

	flowsDir := filepath.Join(tmpDir, "flows")
	queueDir := filepath.Join(tmpDir, "queue")
	schedulesDir := filepath.Join(tmpDir, "schedules")

	reg, err := registry.NewFileFlowRegistry(flowsDir)
	require.NoError(t, err)

	// Add a test flow
	err = reg.Add("test-flow", []byte("runs-on: host\nsteps:\n  - cmd: echo test\n    kind: shell"))
	require.NoError(t, err)

	q, err := queue.NewFileQueue(queueDir, reg, queue.CleanupConfig{})
	require.NoError(t, err)

	store, err := NewFileStore(schedulesDir)
	require.NoError(t, err)

	sched := New(store, q, reg)
	return sched, store, reg
}

func TestValidateScheduleName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid alphanumeric", "mySchedule123", false},
		{"valid with underscore", "my_schedule", false},
		{"valid with hyphen", "my-schedule", false},
		{"valid mixed", "My-Schedule_123", false},
		{"empty", "", true},
		{"with space", "my schedule", true},
		{"with dot", "my.schedule", true},
		{"with slash", "my/schedule", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateScheduleName(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateScheduleType(t *testing.T) {
	tests := []struct {
		name    string
		input   ScheduleType
		wantErr bool
	}{
		{"cron", ScheduleTypeCron, false},
		{"interval", ScheduleTypeInterval, false},
		{"invalid", ScheduleType("invalid"), true},
		{"empty", ScheduleType(""), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateScheduleType(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestFileStore_CRUD(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	t.Run("save and get", func(t *testing.T) {
		sched := &Schedule{
			Name:       "test-schedule",
			Flow:       "test-flow",
			Type:       ScheduleTypeCron,
			Expression: "0 0 * * * *",
			Enabled:    true,
			CreatedAt:  time.Now().UTC(),
		}

		err := store.Save(sched)
		require.NoError(t, err)

		got, err := store.Get("test-schedule")
		require.NoError(t, err)
		assert.Equal(t, sched.Name, got.Name)
		assert.Equal(t, sched.Flow, got.Flow)
		assert.Equal(t, sched.Type, got.Type)
		assert.Equal(t, sched.Expression, got.Expression)
		assert.Equal(t, sched.Enabled, got.Enabled)
	})

	t.Run("exists", func(t *testing.T) {
		assert.True(t, store.Exists("test-schedule"))
		assert.False(t, store.Exists("nonexistent"))
	})

	t.Run("list", func(t *testing.T) {
		// Add another schedule
		sched2 := &Schedule{
			Name:       "another-schedule",
			Flow:       "test-flow",
			Type:       ScheduleTypeInterval,
			Expression: "1h",
			Enabled:    false,
			CreatedAt:  time.Now().UTC(),
		}
		err := store.Save(sched2)
		require.NoError(t, err)

		schedules, err := store.List()
		require.NoError(t, err)
		assert.Len(t, schedules, 2)
		// Should be sorted by name
		assert.Equal(t, "another-schedule", schedules[0].Name)
		assert.Equal(t, "test-schedule", schedules[1].Name)
	})

	t.Run("delete", func(t *testing.T) {
		err := store.Delete("test-schedule")
		require.NoError(t, err)

		_, err = store.Get("test-schedule")
		assert.ErrorIs(t, err, ErrScheduleNotFound)
	})

	t.Run("get nonexistent", func(t *testing.T) {
		_, err := store.Get("nonexistent")
		assert.ErrorIs(t, err, ErrScheduleNotFound)
	})

	t.Run("delete nonexistent", func(t *testing.T) {
		err := store.Delete("nonexistent")
		assert.ErrorIs(t, err, ErrScheduleNotFound)
	})
}

func TestScheduler_Create(t *testing.T) {
	sched, _, _ := setupTestScheduler(t)

	t.Run("create valid cron schedule", func(t *testing.T) {
		schedule := &Schedule{
			Name:       "cron-schedule",
			Flow:       "test-flow",
			Type:       ScheduleTypeCron,
			Expression: "0 0 * * * *", // Every hour
			Enabled:    true,
		}

		err := sched.Create(schedule)
		require.NoError(t, err)

		got, err := sched.Get("cron-schedule")
		require.NoError(t, err)
		assert.Equal(t, "cron-schedule", got.Name)
		assert.NotZero(t, got.CreatedAt)
	})

	t.Run("create valid interval schedule", func(t *testing.T) {
		schedule := &Schedule{
			Name:       "interval-schedule",
			Flow:       "test-flow",
			Type:       ScheduleTypeInterval,
			Expression: "30m",
			Enabled:    true,
		}

		err := sched.Create(schedule)
		require.NoError(t, err)
	})

	t.Run("create disabled schedule", func(t *testing.T) {
		schedule := &Schedule{
			Name:       "disabled-schedule",
			Flow:       "test-flow",
			Type:       ScheduleTypeInterval,
			Expression: "1h",
			Enabled:    false,
		}

		err := sched.Create(schedule)
		require.NoError(t, err)

		got, err := sched.Get("disabled-schedule")
		require.NoError(t, err)
		assert.False(t, got.Enabled)
		assert.Nil(t, got.NextRunAt) // No next run since disabled
	})

	t.Run("create with inputs", func(t *testing.T) {
		schedule := &Schedule{
			Name:       "schedule-with-inputs",
			Flow:       "test-flow",
			Type:       ScheduleTypeInterval,
			Expression: "1h",
			Inputs:     map[string]string{"key": "value"},
			Enabled:    true,
		}

		err := sched.Create(schedule)
		require.NoError(t, err)

		got, err := sched.Get("schedule-with-inputs")
		require.NoError(t, err)
		assert.Equal(t, "value", got.Inputs["key"])
	})

	t.Run("create duplicate fails", func(t *testing.T) {
		schedule := &Schedule{
			Name:       "cron-schedule", // Already exists
			Flow:       "test-flow",
			Type:       ScheduleTypeCron,
			Expression: "0 0 * * * *",
			Enabled:    true,
		}

		err := sched.Create(schedule)
		assert.ErrorIs(t, err, ErrScheduleExists)
	})

	t.Run("create with invalid name fails", func(t *testing.T) {
		schedule := &Schedule{
			Name:       "invalid.name",
			Flow:       "test-flow",
			Type:       ScheduleTypeCron,
			Expression: "0 0 * * * *",
			Enabled:    true,
		}

		err := sched.Create(schedule)
		assert.ErrorIs(t, err, ErrInvalidScheduleName)
	})

	t.Run("create with invalid type fails", func(t *testing.T) {
		schedule := &Schedule{
			Name:       "invalid-type",
			Flow:       "test-flow",
			Type:       ScheduleType("invalid"),
			Expression: "0 0 * * * *",
			Enabled:    true,
		}

		err := sched.Create(schedule)
		assert.ErrorIs(t, err, ErrInvalidScheduleType)
	})

	t.Run("create with invalid cron expression fails", func(t *testing.T) {
		schedule := &Schedule{
			Name:       "invalid-cron",
			Flow:       "test-flow",
			Type:       ScheduleTypeCron,
			Expression: "not a cron expression",
			Enabled:    true,
		}

		err := sched.Create(schedule)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid cron expression")
	})

	t.Run("create with invalid interval fails", func(t *testing.T) {
		schedule := &Schedule{
			Name:       "invalid-interval",
			Flow:       "test-flow",
			Type:       ScheduleTypeInterval,
			Expression: "not an interval",
			Enabled:    true,
		}

		err := sched.Create(schedule)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid interval")
	})

	t.Run("create with nonexistent flow fails", func(t *testing.T) {
		schedule := &Schedule{
			Name:       "nonexistent-flow",
			Flow:       "does-not-exist",
			Type:       ScheduleTypeInterval,
			Expression: "1h",
			Enabled:    true,
		}

		err := sched.Create(schedule)
		assert.ErrorIs(t, err, ErrFlowNotFound)
	})
}

func TestScheduler_Update(t *testing.T) {
	sched, _, _ := setupTestScheduler(t)

	// Create initial schedule
	initial := &Schedule{
		Name:       "update-test",
		Flow:       "test-flow",
		Type:       ScheduleTypeCron,
		Expression: "0 0 * * * *",
		Enabled:    true,
	}
	err := sched.Create(initial)
	require.NoError(t, err)

	t.Run("update expression", func(t *testing.T) {
		updated := &Schedule{
			Name:       "update-test",
			Flow:       "test-flow",
			Type:       ScheduleTypeCron,
			Expression: "0 30 * * * *",
			Enabled:    true,
		}

		err := sched.Update(updated)
		require.NoError(t, err)

		got, err := sched.Get("update-test")
		require.NoError(t, err)
		assert.Equal(t, "0 30 * * * *", got.Expression)
	})

	t.Run("update type", func(t *testing.T) {
		updated := &Schedule{
			Name:       "update-test",
			Flow:       "test-flow",
			Type:       ScheduleTypeInterval,
			Expression: "1h",
			Enabled:    true,
		}

		err := sched.Update(updated)
		require.NoError(t, err)

		got, err := sched.Get("update-test")
		require.NoError(t, err)
		assert.Equal(t, ScheduleTypeInterval, got.Type)
		assert.Equal(t, "1h", got.Expression)
	})

	t.Run("update nonexistent fails", func(t *testing.T) {
		updated := &Schedule{
			Name:       "nonexistent",
			Flow:       "test-flow",
			Type:       ScheduleTypeInterval,
			Expression: "1h",
			Enabled:    true,
		}

		err := sched.Update(updated)
		assert.ErrorIs(t, err, ErrScheduleNotFound)
	})
}

func TestScheduler_Delete(t *testing.T) {
	sched, _, _ := setupTestScheduler(t)

	// Create schedule
	schedule := &Schedule{
		Name:       "delete-test",
		Flow:       "test-flow",
		Type:       ScheduleTypeInterval,
		Expression: "1h",
		Enabled:    true,
	}
	err := sched.Create(schedule)
	require.NoError(t, err)

	t.Run("delete existing", func(t *testing.T) {
		err := sched.Delete("delete-test")
		require.NoError(t, err)

		_, err = sched.Get("delete-test")
		assert.ErrorIs(t, err, ErrScheduleNotFound)
	})

	t.Run("delete nonexistent fails", func(t *testing.T) {
		err := sched.Delete("nonexistent")
		assert.ErrorIs(t, err, ErrScheduleNotFound)
	})
}

func TestScheduler_EnableDisable(t *testing.T) {
	sched, _, _ := setupTestScheduler(t)

	// Start the scheduler so cron entries are registered
	err := sched.Start(context.Background())
	require.NoError(t, err)
	defer sched.Stop()

	// Create disabled schedule
	schedule := &Schedule{
		Name:       "toggle-test",
		Flow:       "test-flow",
		Type:       ScheduleTypeInterval,
		Expression: "1h",
		Enabled:    false,
	}
	err = sched.Create(schedule)
	require.NoError(t, err)

	t.Run("enable schedule", func(t *testing.T) {
		err := sched.Enable("toggle-test")
		require.NoError(t, err)

		got, err := sched.Get("toggle-test")
		require.NoError(t, err)
		assert.True(t, got.Enabled)
		assert.NotNil(t, got.NextRunAt)
	})

	t.Run("disable schedule", func(t *testing.T) {
		err := sched.Disable("toggle-test")
		require.NoError(t, err)

		got, err := sched.Get("toggle-test")
		require.NoError(t, err)
		assert.False(t, got.Enabled)
		assert.Nil(t, got.NextRunAt)
	})

	t.Run("enable nonexistent fails", func(t *testing.T) {
		err := sched.Enable("nonexistent")
		assert.ErrorIs(t, err, ErrScheduleNotFound)
	})

	t.Run("disable nonexistent fails", func(t *testing.T) {
		err := sched.Disable("nonexistent")
		assert.ErrorIs(t, err, ErrScheduleNotFound)
	})
}

func TestScheduler_List(t *testing.T) {
	sched, _, _ := setupTestScheduler(t)

	// Create multiple schedules
	schedules := []*Schedule{
		{Name: "sched-a", Flow: "test-flow", Type: ScheduleTypeInterval, Expression: "1h", Enabled: true},
		{Name: "sched-b", Flow: "test-flow", Type: ScheduleTypeCron, Expression: "0 0 * * * *", Enabled: false},
		{Name: "sched-c", Flow: "test-flow", Type: ScheduleTypeInterval, Expression: "30m", Enabled: true},
	}

	for _, s := range schedules {
		err := sched.Create(s)
		require.NoError(t, err)
	}

	t.Run("list all", func(t *testing.T) {
		list, err := sched.List()
		require.NoError(t, err)
		assert.Len(t, list, 3)

		// Should be sorted by name
		assert.Equal(t, "sched-a", list[0].Name)
		assert.Equal(t, "sched-b", list[1].Name)
		assert.Equal(t, "sched-c", list[2].Name)
	})

	t.Run("list includes next run for enabled", func(t *testing.T) {
		// Start the scheduler to populate next run times
		err := sched.Start(context.Background())
		require.NoError(t, err)
		defer sched.Stop()

		list, err := sched.List()
		require.NoError(t, err)

		for _, s := range list {
			if s.Enabled {
				assert.NotNil(t, s.NextRunAt, "enabled schedule %s should have NextRunAt", s.Name)
			} else {
				assert.Nil(t, s.NextRunAt, "disabled schedule %s should not have NextRunAt", s.Name)
			}
		}
	})
}

func TestScheduler_StartStop(t *testing.T) {
	sched, _, _ := setupTestScheduler(t)

	// Create some schedules
	err := sched.Create(&Schedule{
		Name:       "start-test-1",
		Flow:       "test-flow",
		Type:       ScheduleTypeInterval,
		Expression: "1h",
		Enabled:    true,
	})
	require.NoError(t, err)

	err = sched.Create(&Schedule{
		Name:       "start-test-2",
		Flow:       "test-flow",
		Type:       ScheduleTypeInterval,
		Expression: "2h",
		Enabled:    false,
	})
	require.NoError(t, err)

	t.Run("start loads enabled schedules", func(t *testing.T) {
		err := sched.Start(context.Background())
		require.NoError(t, err)

		// Check enabled schedule has next run
		got, err := sched.Get("start-test-1")
		require.NoError(t, err)
		assert.NotNil(t, got.NextRunAt)

		// Check disabled schedule has no next run
		got, err = sched.Get("start-test-2")
		require.NoError(t, err)
		assert.Nil(t, got.NextRunAt)
	})

	t.Run("stop gracefully", func(t *testing.T) {
		sched.Stop()
		// Should not panic or hang
	})
}

func TestScheduler_ConcurrentAccess(t *testing.T) {
	sched, _, _ := setupTestScheduler(t)

	err := sched.Start(context.Background())
	require.NoError(t, err)
	defer sched.Stop()

	// Create initial schedule
	err = sched.Create(&Schedule{
		Name:       "concurrent-test",
		Flow:       "test-flow",
		Type:       ScheduleTypeInterval,
		Expression: "1h",
		Enabled:    true,
	})
	require.NoError(t, err)

	var wg sync.WaitGroup
	errs := make(chan error, 100)

	// Run concurrent operations
	for i := 0; i < 10; i++ {
		wg.Add(3)

		// Concurrent reads
		go func() {
			defer wg.Done()
			_, err := sched.List()
			if err != nil {
				errs <- err
			}
		}()

		// Concurrent get
		go func() {
			defer wg.Done()
			_, err := sched.Get("concurrent-test")
			if err != nil {
				errs <- err
			}
		}()

		// Concurrent enable/disable
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				sched.Enable("concurrent-test")
			} else {
				sched.Disable("concurrent-test")
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent operation error: %v", err)
	}
}

func TestScheduler_PersistenceAcrossRestart(t *testing.T) {
	tmpDir := t.TempDir()
	schedulesDir := filepath.Join(tmpDir, "schedules")
	flowsDir := filepath.Join(tmpDir, "flows")
	queueDir := filepath.Join(tmpDir, "queue")

	// Create registry and queue
	reg, err := registry.NewFileFlowRegistry(flowsDir)
	require.NoError(t, err)
	err = reg.Add("test-flow", []byte("runs-on: host\nsteps:\n  - cmd: echo test\n    kind: shell"))
	require.NoError(t, err)

	q, err := queue.NewFileQueue(queueDir, reg, queue.CleanupConfig{})
	require.NoError(t, err)

	// Create first scheduler instance
	store1, err := NewFileStore(schedulesDir)
	require.NoError(t, err)
	sched1 := New(store1, q, reg)

	// Create schedule
	err = sched1.Create(&Schedule{
		Name:       "persist-test",
		Flow:       "test-flow",
		Type:       ScheduleTypeInterval,
		Expression: "1h",
		Inputs:     map[string]string{"key": "value"},
		Enabled:    true,
	})
	require.NoError(t, err)

	// Start and stop first scheduler
	err = sched1.Start(context.Background())
	require.NoError(t, err)
	sched1.Stop()

	// Create second scheduler instance (simulating restart)
	store2, err := NewFileStore(schedulesDir)
	require.NoError(t, err)
	sched2 := New(store2, q, reg)

	// Start second scheduler
	err = sched2.Start(context.Background())
	require.NoError(t, err)
	defer sched2.Stop()

	// Verify schedule persisted
	got, err := sched2.Get("persist-test")
	require.NoError(t, err)
	assert.Equal(t, "persist-test", got.Name)
	assert.Equal(t, "test-flow", got.Flow)
	assert.Equal(t, ScheduleTypeInterval, got.Type)
	assert.Equal(t, "1h", got.Expression)
	assert.Equal(t, "value", got.Inputs["key"])
	assert.True(t, got.Enabled)
	assert.NotNil(t, got.NextRunAt) // Should have next run since enabled
}
