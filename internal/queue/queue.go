package queue

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"git.myservermanager.com/varakh/yafe/internal/registry"
	"github.com/rs/zerolog/log"
)

var (
	ErrJobNotFound = errors.New("job not found")
)

type Job struct {
	ID        string            `json:"id"`
	Flow      string            `json:"flow"`
	Inputs    map[string]string `json:"inputs,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	StartedAt *time.Time        `json:"started_at,omitempty"`
	EndedAt   *time.Time        `json:"ended_at,omitempty"`
	Error     string            `json:"error,omitempty"`
	ExitCode  *int              `json:"exit_code,omitempty"`
}

type CleanupConfig struct {
	DoneRetention   time.Duration
	FailedRetention time.Duration
}

// Queue defines the interface for job queue implementations.
type Queue interface {
	Enqueue(flowName string, inputs map[string]string) (*Job, error)
	Dequeue() (*Job, error)
	MarkDone(job *Job, exitCode int) error
	MarkFailed(job *Job, exitCode int, errMsg string) error
	ListJobs(status JobStatus) ([]*Job, error)
	GetJob(jobID string) (*Job, JobStatus, error)
	DeleteJob(jobID string) error
	RunCleanup() (int, error)
	CleanupEnabled() bool
	LogFile(jobID string) string
	GetJobLogs(jobID string) ([]byte, error)
}

// FileQueue is a file-based implementation of Queue.
type FileQueue struct {
	baseDir  string
	registry registry.FlowRegistry
	cleanup  CleanupConfig
}

const logsDirName = "logs"

// NewFileQueue creates a new file-based queue.
func NewFileQueue(baseDir string, reg registry.FlowRegistry, cleanup CleanupConfig) (*FileQueue, error) {
	q := &FileQueue{
		baseDir:  baseDir,
		registry: reg,
		cleanup:  cleanup,
	}

	// Create queue subdirectories
	for _, status := range JobStatusValues() {
		dir := filepath.Join(baseDir, status.String())
		if err := os.MkdirAll(dir, 0750); err != nil {
			return nil, fmt.Errorf("creating queue directory %s: %w", dir, err)
		}
	}

	// Create logs directory
	logsDir := filepath.Join(baseDir, logsDirName)
	if err := os.MkdirAll(logsDir, 0750); err != nil {
		return nil, fmt.Errorf("creating logs directory %s: %w", logsDir, err)
	}

	return q, nil
}

func (q *FileQueue) Enqueue(flowName string, inputs map[string]string) (*Job, error) {
	flowName = strings.TrimSpace(flowName)
	if err := registry.ValidateFlowName(flowName); err != nil {
		return nil, err
	}

	// Verify flow exists
	if !q.registry.Exists(flowName) {
		return nil, fmt.Errorf("%w: %s", registry.ErrFlowNotFound, flowName)
	}

	job := &Job{
		ID:        generateJobID(),
		Flow:      flowName,
		Inputs:    inputs,
		CreatedAt: time.Now().UTC(),
	}

	if err := q.writeJob(JobStatusPending, job); err != nil {
		return nil, err
	}

	return job, nil
}

func (q *FileQueue) Dequeue() (*Job, error) {
	pendingDir := filepath.Join(q.baseDir, JobStatusPending.String())
	entries, err := os.ReadDir(pendingDir)
	if err != nil {
		return nil, fmt.Errorf("reading pending directory: %w", err)
	}

	if len(entries) == 0 {
		return nil, nil
	}

	// Sort by filename (timestamp prefix ensures FIFO)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	// Try to claim a job with file locking
	for _, entry := range entries {
		filename := entry.Name()
		if !strings.HasSuffix(filename, ".json") {
			continue
		}

		job, claimed, err := q.tryClaimJob(filename)
		if err != nil {
			return nil, err
		}
		if claimed {
			return job, nil
		}
		// Job was claimed by another worker, try next
	}

	return nil, nil
}

func (q *FileQueue) tryClaimJob(filename string) (*Job, bool, error) {
	pendingPath := filepath.Join(q.baseDir, JobStatusPending.String(), filename)

	// Open file for locking
	//gosec:disable G304 -- Explicitly allowed to use base location
	f, err := os.Open(pendingPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Already claimed by another worker
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("opening job file: %w", err)
	}
	defer f.Close()

	// Try to acquire exclusive lock (non-blocking)
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		// Another worker has the lock
		return nil, false, nil
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	// Double-check file still exists (race condition protection)
	if _, err := os.Stat(pendingPath); os.IsNotExist(err) {
		return nil, false, nil
	}

	job, err := q.readJob(JobStatusPending, filename)
	if err != nil {
		return nil, false, err
	}

	// Move to running
	now := time.Now().UTC()
	job.StartedAt = &now
	if err := q.moveJob(JobStatusPending, JobStatusRunning, filename, job); err != nil {
		return nil, false, err
	}

	return job, true, nil
}

func (q *FileQueue) MarkDone(job *Job, exitCode int) error {
	now := time.Now().UTC()
	job.EndedAt = &now
	job.ExitCode = &exitCode

	filename := q.jobFilename(job)
	return q.moveJob(JobStatusRunning, JobStatusDone, filename, job)
}

func (q *FileQueue) MarkFailed(job *Job, exitCode int, errMsg string) error {
	now := time.Now().UTC()
	job.EndedAt = &now
	job.ExitCode = &exitCode
	job.Error = errMsg

	filename := q.jobFilename(job)
	return q.moveJob(JobStatusRunning, JobStatusFailed, filename, job)
}

func (q *FileQueue) ListJobs(status JobStatus) ([]*Job, error) {
	dir := filepath.Join(q.baseDir, status.String())
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s directory: %w", status, err)
	}

	var jobs []*Job
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		job, err := q.readJob(status, entry.Name())
		if err != nil {
			continue // Skip malformed jobs
		}
		jobs = append(jobs, job)
	}

	// Sort by created_at ascending
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].CreatedAt.Before(jobs[j].CreatedAt)
	})

	return jobs, nil
}

func (q *FileQueue) GetJob(jobID string) (*Job, JobStatus, error) {
	for _, status := range JobStatusValues() {
		dir := filepath.Join(q.baseDir, status.String())
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			// Extract job ID from filename format: <timestamp>-<jobID>.json
			name := entry.Name()
			if !strings.HasSuffix(name, ".json") {
				continue
			}
			// Find the ID portion after the timestamp prefix
			dashIdx := strings.Index(name, "-")
			if dashIdx == -1 {
				continue
			}
			fileJobID := strings.TrimSuffix(name[dashIdx+1:], ".json")
			if fileJobID == jobID {
				job, err := q.readJob(status, entry.Name())
				if err != nil {
					// File may have been moved to another status directory
					// between ReadDir and readJob - continue searching
					if os.IsNotExist(err) {
						continue
					}
					return nil, "", err
				}
				return job, status, nil
			}
		}
	}
	return nil, "", ErrJobNotFound
}

func (q *FileQueue) DeleteJob(jobID string) error {
	for _, status := range JobStatusValues() {
		dir := filepath.Join(q.baseDir, status.String())
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			name := entry.Name()
			if !strings.HasSuffix(name, ".json") {
				continue
			}
			dashIdx := strings.Index(name, "-")
			if dashIdx == -1 {
				continue
			}
			fileJobID := strings.TrimSuffix(name[dashIdx+1:], ".json")
			if fileJobID == jobID {
				path := filepath.Join(dir, name)
				if err := os.Remove(path); err != nil {
					return fmt.Errorf("deleting job file: %w", err)
				}
				// Also remove log file
				if err := os.Remove(q.LogFile(jobID)); err != nil && !os.IsNotExist(err) {
					log.Warn().Err(err).Str("job_id", jobID).Msg("Failed to remove job log file")
				}
				return nil
			}
		}
	}
	return ErrJobNotFound
}

func (q *FileQueue) jobFilename(job *Job) string {
	return fmt.Sprintf("%d-%s.json", job.CreatedAt.UnixNano(), job.ID)
}

func (q *FileQueue) writeJob(status JobStatus, job *Job) error {
	data, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling job: %w", err)
	}

	filename := q.jobFilename(job)
	targetPath := filepath.Join(q.baseDir, status.String(), filename)

	// Write to temp file first, then atomic rename
	tmpPath := targetPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("writing job file: %w", err)
	}

	if err := os.Rename(tmpPath, targetPath); err != nil {
		if removeErr := os.Remove(tmpPath); removeErr != nil && !os.IsNotExist(removeErr) {
			log.Warn().Err(removeErr).Str("path", tmpPath).Msg("Failed to clean up temp file")
		}
		return fmt.Errorf("renaming job file: %w", err)
	}

	return nil
}

func (q *FileQueue) readJob(status JobStatus, filename string) (*Job, error) {
	path := filepath.Join(q.baseDir, status.String(), filename)
	//gosec:disable G304 -- Explicitly allowed to use base location
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading job file: %w", err)
	}

	var job Job
	if err := json.Unmarshal(data, &job); err != nil {
		return nil, fmt.Errorf("unmarshaling job: %w", err)
	}

	return &job, nil
}

func (q *FileQueue) moveJob(fromStatus, toStatus JobStatus, filename string, job *Job) error {
	// Write updated job to new location first
	data, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling job: %w", err)
	}

	fromPath := filepath.Join(q.baseDir, fromStatus.String(), filename)
	toPath := filepath.Join(q.baseDir, toStatus.String(), filename)
	tmpPath := toPath + ".tmp"

	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("writing job file: %w", err)
	}

	if err := os.Rename(tmpPath, toPath); err != nil {
		if removeErr := os.Remove(tmpPath); removeErr != nil && !os.IsNotExist(removeErr) {
			log.Warn().Err(removeErr).Str("path", tmpPath).Msg("Failed to clean up temp file")
		}
		return fmt.Errorf("moving job to %s: %w", toStatus, err)
	}

	// Remove from old location
	if err := os.Remove(fromPath); err != nil && !os.IsNotExist(err) {
		log.Warn().Err(err).Str("path", fromPath).Msg("Failed to remove old job file")
	}

	return nil
}

// generateJobID creates a random job identifier.
// Panics if crypto/rand fails, as this indicates a system-level issue.
func generateJobID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// RunCleanup removes jobs older than the configured retention periods.
// Returns the number of jobs cleaned up.
func (q *FileQueue) RunCleanup() (int, error) {
	var cleaned int

	if q.cleanup.DoneRetention > 0 {
		n, err := q.cleanupStatus(JobStatusDone, q.cleanup.DoneRetention)
		if err != nil {
			return cleaned, fmt.Errorf("cleaning done jobs: %w", err)
		}
		cleaned += n
	}

	if q.cleanup.FailedRetention > 0 {
		n, err := q.cleanupStatus(JobStatusFailed, q.cleanup.FailedRetention)
		if err != nil {
			return cleaned, fmt.Errorf("cleaning failed jobs: %w", err)
		}
		cleaned += n
	}

	return cleaned, nil
}

func (q *FileQueue) cleanupStatus(status JobStatus, retention time.Duration) (int, error) {
	dir := filepath.Join(q.baseDir, status.String())
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	cutoff := time.Now().Add(-retention)
	var cleaned int

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		job, err := q.readJob(status, entry.Name())
		if err != nil {
			continue
		}

		// Use EndedAt for cleanup decision
		if job.EndedAt != nil && job.EndedAt.Before(cutoff) {
			path := filepath.Join(dir, entry.Name())
			if err := os.Remove(path); err != nil {
				log.Warn().Err(err).Str("path", path).Msg("Failed to remove old job file during cleanup")
				continue
			}
			// Also remove log file
			if err := os.Remove(q.LogFile(job.ID)); err != nil && !os.IsNotExist(err) {
				log.Warn().Err(err).Str("job_id", job.ID).Msg("Failed to remove job log file during cleanup")
			}
			cleaned++
		}
	}

	return cleaned, nil
}

// CleanupEnabled returns true if cleanup is configured.
func (q *FileQueue) CleanupEnabled() bool {
	return q.cleanup.DoneRetention > 0 || q.cleanup.FailedRetention > 0
}

// LogFile returns the path to a job's log file.
func (q *FileQueue) LogFile(jobID string) string {
	return filepath.Join(q.baseDir, logsDirName, jobID+".log")
}

// GetJobLogs returns the contents of a job's log file.
func (q *FileQueue) GetJobLogs(jobID string) ([]byte, error) {
	logPath := q.LogFile(jobID)
	//gosec:disable G304 -- Path is constructed from trusted base directory
	data, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrJobNotFound
		}
		return nil, fmt.Errorf("reading log file: %w", err)
	}
	return data, nil
}
