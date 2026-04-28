package queue

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"git.myservermanager.com/varakh/yafe/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestQueue(t *testing.T) (*FileQueue, *registry.FileFlowRegistry) {
	t.Helper()
	tmpDir := t.TempDir()

	flowsDir := filepath.Join(tmpDir, "flows")
	queueDir := filepath.Join(tmpDir, "queue")

	reg, err := registry.NewFileFlowRegistry(flowsDir)
	require.NoError(t, err)

	q, err := NewFileQueue(queueDir, reg, CleanupConfig{})
	require.NoError(t, err)

	return q, reg
}

func setupTestQueueWithCleanup(t *testing.T, cleanup CleanupConfig) (*FileQueue, *registry.FileFlowRegistry) {
	t.Helper()
	tmpDir := t.TempDir()

	flowsDir := filepath.Join(tmpDir, "flows")
	queueDir := filepath.Join(tmpDir, "queue")

	reg, err := registry.NewFileFlowRegistry(flowsDir)
	require.NoError(t, err)

	q, err := NewFileQueue(queueDir, reg, cleanup)
	require.NoError(t, err)

	return q, reg
}

func addTestFlow(t *testing.T, reg *registry.FileFlowRegistry, name string) {
	t.Helper()
	content := []byte("runs-on: host\nsteps:\n  - cmd: echo test\n    kind: shell")
	require.NoError(t, reg.Add(name, content))
}

func TestNewFileQueue(t *testing.T) {
	t.Run("creates queue directories", func(t *testing.T) {
		tmpDir := t.TempDir()
		queueDir := filepath.Join(tmpDir, "queue")
		flowsDir := filepath.Join(tmpDir, "flows")

		reg, err := registry.NewFileFlowRegistry(flowsDir)
		require.NoError(t, err)

		q, err := NewFileQueue(queueDir, reg, CleanupConfig{})

		require.NoError(t, err)
		assert.NotNil(t, q)

		// Verify status directories exist
		for _, status := range JobStatusValues() {
			assert.DirExists(t, filepath.Join(queueDir, status.String()))
		}
	})
}

func TestFileQueue_Enqueue(t *testing.T) {
	t.Run("creates job in pending directory", func(t *testing.T) {
		q, reg := setupTestQueue(t)
		addTestFlow(t, reg, "myflow")

		job, err := q.Enqueue("myflow", nil)

		require.NoError(t, err)
		require.NotNil(t, job)
		assert.NotEmpty(t, job.ID)
		assert.Equal(t, "myflow", job.Flow)
		assert.NotZero(t, job.CreatedAt)
		assert.Nil(t, job.StartedAt)
		assert.Nil(t, job.EndedAt)
	})

	t.Run("stores inputs", func(t *testing.T) {
		q, reg := setupTestQueue(t)
		addTestFlow(t, reg, "myflow")
		inputs := map[string]string{"key1": "value1", "key2": "value2"}

		job, err := q.Enqueue("myflow", inputs)

		require.NoError(t, err)
		assert.Equal(t, inputs, job.Inputs)
	})

	t.Run("returns error for nonexistent flow", func(t *testing.T) {
		q, _ := setupTestQueue(t)

		job, err := q.Enqueue("nonexistent", nil)

		require.Error(t, err)
		assert.ErrorIs(t, err, registry.ErrFlowNotFound)
		assert.Nil(t, job)
	})

	t.Run("returns error for invalid flow name", func(t *testing.T) {
		q, _ := setupTestQueue(t)

		job, err := q.Enqueue("invalid/name", nil)

		require.Error(t, err)
		assert.ErrorIs(t, err, registry.ErrInvalidFlowName)
		assert.Nil(t, job)
	})

	t.Run("trims whitespace from flow name", func(t *testing.T) {
		q, reg := setupTestQueue(t)
		addTestFlow(t, reg, "myflow")

		job, err := q.Enqueue("  myflow  ", nil)

		require.NoError(t, err)
		assert.Equal(t, "myflow", job.Flow)
	})
}

func TestFileQueue_Dequeue(t *testing.T) {
	t.Run("returns nil when queue is empty", func(t *testing.T) {
		q, _ := setupTestQueue(t)

		job, err := q.Dequeue()

		require.NoError(t, err)
		assert.Nil(t, job)
	})

	t.Run("returns oldest pending job first", func(t *testing.T) {
		q, reg := setupTestQueue(t)
		addTestFlow(t, reg, "flow1")
		addTestFlow(t, reg, "flow2")

		job1, err := q.Enqueue("flow1", nil)
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond) // Ensure different timestamps
		_, err = q.Enqueue("flow2", nil)
		require.NoError(t, err)

		dequeued, err := q.Dequeue()

		require.NoError(t, err)
		require.NotNil(t, dequeued)
		assert.Equal(t, job1.ID, dequeued.ID)
	})

	t.Run("moves job to running status", func(t *testing.T) {
		q, reg := setupTestQueue(t)
		addTestFlow(t, reg, "myflow")

		job, err := q.Enqueue("myflow", nil)
		require.NoError(t, err)

		dequeued, err := q.Dequeue()
		require.NoError(t, err)

		// Verify job is now in running
		foundJob, status, err := q.GetJob(job.ID)
		require.NoError(t, err)
		assert.Equal(t, JobStatusRunning, status)
		assert.NotNil(t, foundJob.StartedAt)
		assert.Equal(t, dequeued.ID, foundJob.ID)
	})

	t.Run("subsequent dequeue returns next job", func(t *testing.T) {
		q, reg := setupTestQueue(t)
		addTestFlow(t, reg, "flow1")
		addTestFlow(t, reg, "flow2")

		_, err := q.Enqueue("flow1", nil)
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)
		job2, err := q.Enqueue("flow2", nil)
		require.NoError(t, err)

		// Dequeue first job
		_, err = q.Dequeue()
		require.NoError(t, err)

		// Dequeue second job
		dequeued, err := q.Dequeue()
		require.NoError(t, err)
		assert.Equal(t, job2.ID, dequeued.ID)
	})
}

func TestFileQueue_MarkDone(t *testing.T) {
	t.Run("moves job to done status", func(t *testing.T) {
		q, reg := setupTestQueue(t)
		addTestFlow(t, reg, "myflow")

		job, err := q.Enqueue("myflow", nil)
		require.NoError(t, err)

		job, err = q.Dequeue()
		require.NoError(t, err)

		err = q.MarkDone(job, 0)
		require.NoError(t, err)

		// Verify job is now done
		foundJob, status, err := q.GetJob(job.ID)
		require.NoError(t, err)
		assert.Equal(t, JobStatusDone, status)
		assert.NotNil(t, foundJob.EndedAt)
		assert.NotNil(t, foundJob.ExitCode)
		assert.Equal(t, 0, *foundJob.ExitCode)
	})

	t.Run("stores non-zero exit code", func(t *testing.T) {
		q, reg := setupTestQueue(t)
		addTestFlow(t, reg, "myflow")

		job, _ := q.Enqueue("myflow", nil)
		job, _ = q.Dequeue()

		err := q.MarkDone(job, 42)
		require.NoError(t, err)

		foundJob, _, _ := q.GetJob(job.ID)
		assert.Equal(t, 42, *foundJob.ExitCode)
	})
}

func TestFileQueue_MarkFailed(t *testing.T) {
	t.Run("moves job to failed status", func(t *testing.T) {
		q, reg := setupTestQueue(t)
		addTestFlow(t, reg, "myflow")

		job, _ := q.Enqueue("myflow", nil)
		job, _ = q.Dequeue()

		err := q.MarkFailed(job, 1, "something went wrong")
		require.NoError(t, err)

		foundJob, status, err := q.GetJob(job.ID)
		require.NoError(t, err)
		assert.Equal(t, JobStatusFailed, status)
		assert.NotNil(t, foundJob.EndedAt)
		assert.Equal(t, 1, *foundJob.ExitCode)
		assert.Equal(t, "something went wrong", foundJob.Error)
	})
}

func TestFileQueue_ListJobs(t *testing.T) {
	t.Run("returns empty list when no jobs", func(t *testing.T) {
		q, _ := setupTestQueue(t)

		jobs, err := q.ListJobs(JobStatusPending)

		require.NoError(t, err)
		assert.Empty(t, jobs)
	})

	t.Run("returns jobs sorted by created_at ascending", func(t *testing.T) {
		q, reg := setupTestQueue(t)
		addTestFlow(t, reg, "myflow")

		job1, _ := q.Enqueue("myflow", nil)
		time.Sleep(10 * time.Millisecond)
		job2, _ := q.Enqueue("myflow", nil)
		time.Sleep(10 * time.Millisecond)
		job3, _ := q.Enqueue("myflow", nil)

		jobs, err := q.ListJobs(JobStatusPending)

		require.NoError(t, err)
		require.Len(t, jobs, 3)
		assert.Equal(t, job1.ID, jobs[0].ID)
		assert.Equal(t, job2.ID, jobs[1].ID)
		assert.Equal(t, job3.ID, jobs[2].ID)
	})

	t.Run("filters by status", func(t *testing.T) {
		q, reg := setupTestQueue(t)
		addTestFlow(t, reg, "myflow")

		// Create jobs in different statuses
		_, _ = q.Enqueue("myflow", nil)
		job2, _ := q.Enqueue("myflow", nil)
		job2, _ = q.Dequeue()   // Move to running
		_ = q.MarkDone(job2, 0) // Move to done

		pendingJobs, err := q.ListJobs(JobStatusPending)
		require.NoError(t, err)
		// One was dequeued, so one remains
		assert.Len(t, pendingJobs, 1)

		doneJobs, err := q.ListJobs(JobStatusDone)
		require.NoError(t, err)
		assert.Len(t, doneJobs, 1)
		assert.Equal(t, job2.ID, doneJobs[0].ID)
	})
}

func TestFileQueue_GetJob(t *testing.T) {
	t.Run("finds job in pending status", func(t *testing.T) {
		q, reg := setupTestQueue(t)
		addTestFlow(t, reg, "myflow")

		job, _ := q.Enqueue("myflow", nil)

		found, status, err := q.GetJob(job.ID)

		require.NoError(t, err)
		assert.Equal(t, JobStatusPending, status)
		assert.Equal(t, job.ID, found.ID)
	})

	t.Run("finds job in running status", func(t *testing.T) {
		q, reg := setupTestQueue(t)
		addTestFlow(t, reg, "myflow")

		job, _ := q.Enqueue("myflow", nil)
		job, _ = q.Dequeue()

		found, status, err := q.GetJob(job.ID)

		require.NoError(t, err)
		assert.Equal(t, JobStatusRunning, status)
		assert.Equal(t, job.ID, found.ID)
	})

	t.Run("finds job in done status", func(t *testing.T) {
		q, reg := setupTestQueue(t)
		addTestFlow(t, reg, "myflow")

		job, _ := q.Enqueue("myflow", nil)
		job, _ = q.Dequeue()
		_ = q.MarkDone(job, 0)

		found, status, err := q.GetJob(job.ID)

		require.NoError(t, err)
		assert.Equal(t, JobStatusDone, status)
		assert.Equal(t, job.ID, found.ID)
	})

	t.Run("finds job in failed status", func(t *testing.T) {
		q, reg := setupTestQueue(t)
		addTestFlow(t, reg, "myflow")

		job, _ := q.Enqueue("myflow", nil)
		job, _ = q.Dequeue()
		_ = q.MarkFailed(job, 1, "error")

		found, status, err := q.GetJob(job.ID)

		require.NoError(t, err)
		assert.Equal(t, JobStatusFailed, status)
		assert.Equal(t, job.ID, found.ID)
	})

	t.Run("returns error for nonexistent job", func(t *testing.T) {
		q, _ := setupTestQueue(t)

		_, _, err := q.GetJob("nonexistent")

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrJobNotFound)
	})
}

func TestFileQueue_DeleteJob(t *testing.T) {
	t.Run("deletes pending job", func(t *testing.T) {
		q, reg := setupTestQueue(t)
		addTestFlow(t, reg, "myflow")

		job, _ := q.Enqueue("myflow", nil)

		err := q.DeleteJob(job.ID)

		require.NoError(t, err)

		_, _, err = q.GetJob(job.ID)
		assert.ErrorIs(t, err, ErrJobNotFound)
	})

	t.Run("deletes running job", func(t *testing.T) {
		q, reg := setupTestQueue(t)
		addTestFlow(t, reg, "myflow")

		job, _ := q.Enqueue("myflow", nil)
		job, _ = q.Dequeue()

		err := q.DeleteJob(job.ID)

		require.NoError(t, err)
		_, _, err = q.GetJob(job.ID)
		assert.ErrorIs(t, err, ErrJobNotFound)
	})

	t.Run("deletes done job", func(t *testing.T) {
		q, reg := setupTestQueue(t)
		addTestFlow(t, reg, "myflow")

		job, _ := q.Enqueue("myflow", nil)
		job, _ = q.Dequeue()
		_ = q.MarkDone(job, 0)

		err := q.DeleteJob(job.ID)

		require.NoError(t, err)
		_, _, err = q.GetJob(job.ID)
		assert.ErrorIs(t, err, ErrJobNotFound)
	})

	t.Run("returns error for nonexistent job", func(t *testing.T) {
		q, _ := setupTestQueue(t)

		err := q.DeleteJob("nonexistent")

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrJobNotFound)
	})
}

func TestFileQueue_Cleanup(t *testing.T) {
	t.Run("removes old done jobs", func(t *testing.T) {
		q, reg := setupTestQueueWithCleanup(t, CleanupConfig{
			DoneRetention: 1 * time.Millisecond,
		})
		addTestFlow(t, reg, "myflow")

		job, _ := q.Enqueue("myflow", nil)
		job, _ = q.Dequeue()
		_ = q.MarkDone(job, 0)

		// Wait for retention period
		time.Sleep(10 * time.Millisecond)

		cleaned, err := q.RunCleanup()

		require.NoError(t, err)
		assert.Equal(t, 1, cleaned)

		// Verify job is gone
		_, _, err = q.GetJob(job.ID)
		assert.ErrorIs(t, err, ErrJobNotFound)
	})

	t.Run("removes old failed jobs", func(t *testing.T) {
		q, reg := setupTestQueueWithCleanup(t, CleanupConfig{
			FailedRetention: 1 * time.Millisecond,
		})
		addTestFlow(t, reg, "myflow")

		job, _ := q.Enqueue("myflow", nil)
		job, _ = q.Dequeue()
		_ = q.MarkFailed(job, 1, "error")

		// Wait for retention period
		time.Sleep(10 * time.Millisecond)

		cleaned, err := q.RunCleanup()

		require.NoError(t, err)
		assert.Equal(t, 1, cleaned)

		_, _, err = q.GetJob(job.ID)
		assert.ErrorIs(t, err, ErrJobNotFound)
	})

	t.Run("keeps recent jobs", func(t *testing.T) {
		q, reg := setupTestQueueWithCleanup(t, CleanupConfig{
			DoneRetention: 1 * time.Hour,
		})
		addTestFlow(t, reg, "myflow")

		job, _ := q.Enqueue("myflow", nil)
		job, _ = q.Dequeue()
		_ = q.MarkDone(job, 0)

		cleaned, err := q.RunCleanup()

		require.NoError(t, err)
		assert.Equal(t, 0, cleaned)

		// Job should still exist
		_, _, err = q.GetJob(job.ID)
		require.NoError(t, err)
	})

	t.Run("does nothing when cleanup not configured", func(t *testing.T) {
		q, reg := setupTestQueue(t)
		addTestFlow(t, reg, "myflow")

		job, _ := q.Enqueue("myflow", nil)
		job, _ = q.Dequeue()
		_ = q.MarkDone(job, 0)

		cleaned, err := q.RunCleanup()

		require.NoError(t, err)
		assert.Equal(t, 0, cleaned)
	})
}

func TestFileQueue_CleanupEnabled(t *testing.T) {
	t.Run("returns false when no retention configured", func(t *testing.T) {
		q, _ := setupTestQueue(t)
		assert.False(t, q.CleanupEnabled())
	})

	t.Run("returns true when done retention configured", func(t *testing.T) {
		q, _ := setupTestQueueWithCleanup(t, CleanupConfig{
			DoneRetention: 1 * time.Hour,
		})
		assert.True(t, q.CleanupEnabled())
	})

	t.Run("returns true when failed retention configured", func(t *testing.T) {
		q, _ := setupTestQueueWithCleanup(t, CleanupConfig{
			FailedRetention: 1 * time.Hour,
		})
		assert.True(t, q.CleanupEnabled())
	})
}

func TestFileQueue_LogFile(t *testing.T) {
	t.Run("returns correct log file path", func(t *testing.T) {
		q, _ := setupTestQueue(t)

		logPath := q.LogFile("abc123")

		assert.Contains(t, logPath, "logs")
		assert.Contains(t, logPath, "abc123.log")
	})
}

func TestFileQueue_GetJobLogs(t *testing.T) {
	t.Run("returns log contents", func(t *testing.T) {
		q, reg := setupTestQueue(t)
		addTestFlow(t, reg, "myflow")

		job, _ := q.Enqueue("myflow", nil)

		// Write some log content
		logPath := q.LogFile(job.ID)
		require.NoError(t, writeTestFile(logPath, "test log output\n"))

		logs, err := q.GetJobLogs(job.ID)

		require.NoError(t, err)
		assert.Equal(t, "test log output\n", string(logs))
	})

	t.Run("returns error for nonexistent log", func(t *testing.T) {
		q, _ := setupTestQueue(t)

		_, err := q.GetJobLogs("nonexistent")

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrJobNotFound)
	})
}

func TestFileQueue_DeleteJob_RemovesLogs(t *testing.T) {
	t.Run("deletes log file when job is deleted", func(t *testing.T) {
		q, reg := setupTestQueue(t)
		addTestFlow(t, reg, "myflow")

		job, _ := q.Enqueue("myflow", nil)

		// Write a log file
		logPath := q.LogFile(job.ID)
		require.NoError(t, writeTestFile(logPath, "test log"))
		assert.FileExists(t, logPath)

		err := q.DeleteJob(job.ID)
		require.NoError(t, err)

		// Log file should be deleted
		assert.NoFileExists(t, logPath)
	})
}

func TestFileQueue_Cleanup_RemovesLogs(t *testing.T) {
	t.Run("removes log files during cleanup", func(t *testing.T) {
		q, reg := setupTestQueueWithCleanup(t, CleanupConfig{
			DoneRetention: 1 * time.Millisecond,
		})
		addTestFlow(t, reg, "myflow")

		job, _ := q.Enqueue("myflow", nil)
		job, _ = q.Dequeue()
		_ = q.MarkDone(job, 0)

		// Write a log file
		logPath := q.LogFile(job.ID)
		require.NoError(t, writeTestFile(logPath, "test log"))
		assert.FileExists(t, logPath)

		// Wait for retention period
		time.Sleep(10 * time.Millisecond)

		cleaned, err := q.RunCleanup()
		require.NoError(t, err)
		assert.Equal(t, 1, cleaned)

		// Log file should be deleted
		assert.NoFileExists(t, logPath)
	})
}

func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}
