package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"git.myservermanager.com/varakh/yafe/internal/auth"
	"git.myservermanager.com/varakh/yafe/internal/queue"
	"git.myservermanager.com/varakh/yafe/internal/registry"
	"git.myservermanager.com/varakh/yafe/internal/scheduler"
	"github.com/rs/zerolog/log"
)

const (
	shutdownTimeout   = 30 * time.Second
	readHeaderTimeout = 60 * time.Second
	contentTypeJSON   = "application/json"

	// defaultCspValue is the CSP directive string applied when CspEnabled is true
	// and CspValue is not overridden. Suitable for a single-origin Vite/React SPA.
	defaultCspValue = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data:; font-src 'self'; object-src 'none'; frame-ancestors 'none'; frame-src 'self'"
)

// SecurityHeadersConfig holds configuration for security response headers on the web interface.
type SecurityHeadersConfig struct {
	CspEnabled            bool
	CspValue              string
	HstsEnabled           bool
	HstsMaxAge            time.Duration
	HstsIncludeSubDomains bool
	HstsPreload           bool
}

// Config holds server configuration.
type Config struct {
	SocketPath string // Unix socket path (required)
	HTTPAddr   string // HTTP address (optional, empty = disabled)

	// Auth configuration
	SocketAuth bool               // Require auth for Unix socket
	HTTPAuth   bool               // Require auth for HTTP
	Auth       auth.Authenticator // nil = no auth available

	// SecurityHeaders for the web interface
	SecurityHeaders SecurityHeadersConfig
}

type Server struct {
	queue         queue.Queue
	registry      registry.FlowRegistry
	scheduler     scheduler.Service
	socketHandler http.Handler
	httpHandler   http.Handler
	config        Config
	frontendFS    fs.FS
}

func New(q queue.Queue, reg registry.FlowRegistry, sched scheduler.Service, config Config, frontendFS fs.FS) *Server {
	s := &Server{
		queue:      q,
		registry:   reg,
		scheduler:  sched,
		config:     config,
		frontendFS: frontendFS,
	}

	// Build handlers with role requirements
	s.socketHandler = s.buildHandler(config.Auth, config.SocketAuth)
	s.httpHandler = s.buildHandler(config.Auth, config.HTTPAuth)

	return s
}

// buildHandler creates the HTTP handler with appropriate auth middleware.
func (s *Server) buildHandler(authenticator auth.Authenticator, required bool) http.Handler {
	// API mux with auth
	apiMux := http.NewServeMux()

	// Jobs endpoints
	apiMux.Handle("GET /api/v1/jobs",
		auth.RequireRole(auth.RoleJobsread)(http.HandlerFunc(s.handleListJobs)))
	apiMux.Handle("POST /api/v1/jobs",
		auth.RequireRole(auth.RoleJobswrite)(http.HandlerFunc(s.handleEnqueue)))
	apiMux.Handle("GET /api/v1/jobs/{id}",
		auth.RequireRole(auth.RoleJobsread)(http.HandlerFunc(s.handleGetJob)))
	apiMux.Handle("GET /api/v1/jobs/{id}/logs",
		auth.RequireRole(auth.RoleJobsread)(http.HandlerFunc(s.handleGetJobLogs)))
	apiMux.Handle("DELETE /api/v1/jobs/{id}",
		auth.RequireRole(auth.RoleJobswrite)(http.HandlerFunc(s.handleDequeue)))

	// Flows endpoints
	apiMux.Handle("GET /api/v1/flows",
		auth.RequireRole(auth.RoleFlowsread)(http.HandlerFunc(s.handleListFlows)))
	apiMux.Handle("GET /api/v1/flows/{name}",
		auth.RequireRole(auth.RoleFlowsread)(http.HandlerFunc(s.handleGetFlow)))
	apiMux.Handle("PUT /api/v1/flows/{name}",
		auth.RequireRole(auth.RoleFlowswrite)(http.HandlerFunc(s.handleAddFlow)))
	apiMux.Handle("DELETE /api/v1/flows/{name}",
		auth.RequireRole(auth.RoleFlowswrite)(http.HandlerFunc(s.handleDeleteFlow)))
	apiMux.Handle("POST /api/v1/flows/{name}/rename",
		auth.RequireRole(auth.RoleFlowswrite)(http.HandlerFunc(s.handleRenameFlow)))

	// Profile endpoint (no specific role required, just valid auth)
	apiMux.HandleFunc("GET /api/v1/profile", s.handleProfile)

	// Schedules endpoints
	apiMux.Handle("GET /api/v1/schedules",
		auth.RequireRole(auth.RoleSchedulesread)(http.HandlerFunc(s.handleListSchedules)))
	apiMux.Handle("GET /api/v1/schedules/{name}",
		auth.RequireRole(auth.RoleSchedulesread)(http.HandlerFunc(s.handleGetSchedule)))
	apiMux.Handle("POST /api/v1/schedules",
		auth.RequireRole(auth.RoleScheduleswrite)(http.HandlerFunc(s.handleCreateSchedule)))
	apiMux.Handle("PUT /api/v1/schedules/{name}",
		auth.RequireRole(auth.RoleScheduleswrite)(http.HandlerFunc(s.handleUpdateSchedule)))
	apiMux.Handle("DELETE /api/v1/schedules/{name}",
		auth.RequireRole(auth.RoleScheduleswrite)(http.HandlerFunc(s.handleDeleteSchedule)))
	apiMux.Handle("POST /api/v1/schedules/{name}/enable",
		auth.RequireRole(auth.RoleScheduleswrite)(http.HandlerFunc(s.handleEnableSchedule)))
	apiMux.Handle("POST /api/v1/schedules/{name}/disable",
		auth.RequireRole(auth.RoleScheduleswrite)(http.HandlerFunc(s.handleDisableSchedule)))

	// Apply auth middleware and access logging to API routes
	var apiHandler http.Handler = apiMux
	apiHandler = auth.LogAccess(apiHandler)
	apiHandler = auth.Middleware(authenticator, required)(apiHandler)

	// Root mux combines API (with auth) and static files (no auth)
	rootMux := http.NewServeMux()
	rootMux.Handle("/api/", apiHandler)

	// Serve embedded frontend if available
	if s.frontendFS != nil {
		rootMux.Handle("/", securityHeadersMiddleware(s.config.SecurityHeaders, spaFileServer(s.frontendFS)))
	}

	return rootMux
}

// securityHeadersMiddleware sets Content-Security-Policy and Strict-Transport-Security
// headers for the web interface when the respective options are enabled.
func securityHeadersMiddleware(cfg SecurityHeadersConfig, next http.Handler) http.Handler {
	cspValue := cfg.CspValue
	if cspValue == "" {
		cspValue = defaultCspValue
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cfg.CspEnabled {
			w.Header().Set("Content-Security-Policy", cspValue)
		}
		if cfg.HstsEnabled {
			hstsValue := fmt.Sprintf("max-age=%d", int(cfg.HstsMaxAge.Seconds()))
			if cfg.HstsIncludeSubDomains {
				hstsValue += "; includeSubDomains"
			}
			if cfg.HstsPreload {
				hstsValue += "; preload"
			}
			w.Header().Set("Strict-Transport-Security", hstsValue)
		}
		next.ServeHTTP(w, r)
	})
}

// spaFileServer serves static files with SPA fallback to index.html.
func spaFileServer(fsys fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		// Try to open the file
		f, err := fsys.Open(path)
		if err == nil {
			// File exists, check if it's a directory
			stat, err := f.Stat()
			f.Close() //nolint:errcheck,gosec
			if err == nil && !stat.IsDir() {
				// It's a file - serve it normally
				http.FileServerFS(fsys).ServeHTTP(w, r)
				return
			}
		}

		// File not found - check if this is an asset request
		// Assets (with file extensions) should return 404, not fallback to index.html
		if strings.Contains(path, ".") {
			// Has a file extension - likely an asset that's missing
			http.NotFound(w, r)
			return
		}

		// No file extension - serve index.html for SPA routing
		indexPath := "index.html"
		indexFile, err := fsys.Open(indexPath)
		if err != nil {
			http.Error(w, "index.html not found", http.StatusNotFound)
			return
		}
		defer indexFile.Close() //nolint:errcheck

		stat, err := indexFile.Stat()
		if err != nil {
			http.Error(w, "failed to stat index.html", http.StatusInternalServerError)
			return
		}

		http.ServeContent(w, r, indexPath, stat.ModTime(), indexFile.(io.ReadSeeker))
	})
}

func (s *Server) Run(ctx context.Context) error {
	var unixServer *http.Server
	var unixListener net.Listener
	var httpServer *http.Server
	var httpListener net.Listener
	var err error

	// Channel to collect server errors
	errCh := make(chan error, 2)

	// Start Unix socket server if configured
	if s.config.SocketPath != "" {
		// Create socket directory
		socketDir := filepath.Dir(s.config.SocketPath)
		if err := os.MkdirAll(socketDir, 0700); err != nil {
			return err
		}

		// Remove stale socket file
		if err := os.Remove(s.config.SocketPath); err != nil && !os.IsNotExist(err) {
			return err
		}

		// Listen on Unix socket
		lc := &net.ListenConfig{}
		unixListener, err = lc.Listen(ctx, "unix", s.config.SocketPath)
		if err != nil {
			return err
		}
		defer os.Remove(s.config.SocketPath) //nolint:errcheck

		log.Info().Str("socket", s.config.SocketPath).Msg("Listening on Unix socket")
		unixServer = &http.Server{Handler: s.socketHandler} //nolint:gosec

		go func() {
			if err := unixServer.Serve(unixListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- err
			}
		}()
	}

	// Start HTTP server if configured
	if s.config.HTTPAddr != "" {
		lc := &net.ListenConfig{}
		httpListener, err = lc.Listen(ctx, "tcp", s.config.HTTPAddr)
		if err != nil {
			if unixListener != nil {
				unixListener.Close() //nolint:errcheck,gosec
			}
			return err
		}
		httpServer = &http.Server{Handler: s.httpHandler, ReadHeaderTimeout: readHeaderTimeout}
		log.Info().Str("addr", s.config.HTTPAddr).Msg("Listening on HTTP")

		go func() {
			if err := httpServer.Serve(httpListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- err
			}
		}()
	}

	// Wait for shutdown signal or error
	select {
	case <-ctx.Done():
		log.Info().Msg("Server shutting down")
	case err := <-errCh:
		return err
	}

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	var shutdownErr error
	if unixServer != nil {
		if err := unixServer.Shutdown(shutdownCtx); err != nil {
			shutdownErr = err
		}
	}
	if httpServer != nil {
		if err := httpServer.Shutdown(shutdownCtx); err != nil && shutdownErr == nil {
			shutdownErr = err
		}
	}

	return shutdownErr
}

type EnqueueRequest struct {
	Flow   string            `json:"flow"`
	Inputs map[string]string `json:"inputs,omitempty"`
}

type EnqueueResponse struct {
	JobID string `json:"job_id"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func (s *Server) handleEnqueue(w http.ResponseWriter, r *http.Request) {
	var req EnqueueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Flow == "" {
		s.writeError(w, http.StatusBadRequest, "flow is required")
		return
	}

	job, err := s.queue.Enqueue(req.Flow, req.Inputs)
	if err != nil {
		if errors.Is(err, registry.ErrFlowNotFound) {
			s.writeError(w, http.StatusNotFound, err.Error())
			return
		}
		log.Error().Err(err).Msg("Failed to enqueue job")
		s.writeError(w, http.StatusInternalServerError, "failed to enqueue job")
		return
	}

	log.Info().
		Str("job_id", job.ID).
		Str("flow", job.Flow).
		Msg("Job enqueued")

	s.writeJSON(w, http.StatusAccepted, EnqueueResponse{JobID: job.ID})
}

type jobWithStatus struct {
	*queue.Job
	Status queue.JobStatus `json:"status"`
}

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	statusParams := r.URL.Query()["status"]

	// Default to all statuses if none specified
	var statuses []queue.JobStatus
	if len(statusParams) == 0 {
		statuses = queue.JobStatusValues()
	} else {
		for _, param := range statusParams {
			status, err := queue.ParseJobStatus(param)
			if err != nil {
				s.writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			statuses = append(statuses, status)
		}
	}

	// Collect jobs from all requested statuses
	var jobs []jobWithStatus
	for _, status := range statuses {
		statusJobs, err := s.queue.ListJobs(status)
		if err != nil {
			log.Error().Err(err).Msg("Failed to list jobs")
			s.writeError(w, http.StatusInternalServerError, "failed to list jobs")
			return
		}
		for _, job := range statusJobs {
			jobs = append(jobs, jobWithStatus{Job: job, Status: status})
		}
	}

	// Sort by created_at descending (newest first)
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].CreatedAt.After(jobs[j].CreatedAt)
	})

	if jobs == nil {
		jobs = []jobWithStatus{}
	}

	s.writeJSON(w, http.StatusOK, jobs)
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	if jobID == "" {
		s.writeError(w, http.StatusBadRequest, "job id is required")
		return
	}

	job, status, err := s.queue.GetJob(jobID)
	if err != nil {
		if errors.Is(err, queue.ErrJobNotFound) {
			s.writeError(w, http.StatusNotFound, "job not found")
			return
		}
		log.Error().Err(err).Msg("Failed to get job")
		s.writeError(w, http.StatusInternalServerError, "failed to get job")
		return
	}

	response := struct {
		*queue.Job
		Status queue.JobStatus `json:"status"`
	}{
		Job:    job,
		Status: status,
	}

	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleGetJobLogs(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	if jobID == "" {
		s.writeError(w, http.StatusBadRequest, "job id is required")
		return
	}

	// Check if job exists and get its status
	_, status, err := s.queue.GetJob(jobID)
	if err != nil {
		if errors.Is(err, queue.ErrJobNotFound) {
			s.writeError(w, http.StatusNotFound, "job not found")
			return
		}
		log.Error().Err(err).Msg("Failed to get job")
		s.writeError(w, http.StatusInternalServerError, "failed to get job")
		return
	}

	// Logs are only available after job completion
	if status == queue.JobStatusPending || status == queue.JobStatusRunning {
		s.writeError(w, http.StatusTooEarly, "job logs not yet available")
		return
	}

	logs, err := s.queue.GetJobLogs(jobID)
	if err != nil {
		if errors.Is(err, queue.ErrJobNotFound) {
			s.writeError(w, http.StatusNotFound, "job logs not found")
			return
		}
		log.Error().Err(err).Msg("Failed to get job logs")
		s.writeError(w, http.StatusInternalServerError, "failed to get job logs")
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(logs); err != nil { //nolint:gosec
		log.Warn().Err(err).Str("job_id", jobID).Msg("Failed to write job logs response")
	}
}

func (s *Server) handleDequeue(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	if jobID == "" {
		s.writeError(w, http.StatusBadRequest, "job id is required")
		return
	}

	if err := s.queue.DeleteJob(jobID); err != nil {
		if errors.Is(err, queue.ErrJobNotFound) {
			s.writeError(w, http.StatusNotFound, "job not found")
			return
		}
		log.Error().Err(err).Msg("Failed to delete job")
		s.writeError(w, http.StatusInternalServerError, "failed to delete job")
		return
	}

	log.Info().Str("job_id", jobID).Msg("Job deleted")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) writeJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Error().Err(err).Msg("Failed to encode JSON response")
	}
}

func (s *Server) writeError(w http.ResponseWriter, code int, message string) {
	s.writeJSON(w, code, ErrorResponse{Error: message})
}

type FlowResponse struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

func (s *Server) handleListFlows(w http.ResponseWriter, _ *http.Request) {
	flows, err := s.registry.List()
	if err != nil {
		log.Error().Err(err).Msg("Failed to list flows")
		s.writeError(w, http.StatusInternalServerError, "failed to list flows")
		return
	}

	s.writeJSON(w, http.StatusOK, flows)
}

func (s *Server) handleGetFlow(w http.ResponseWriter, r *http.Request) {
	flowName := r.PathValue("name")
	if flowName == "" {
		s.writeError(w, http.StatusBadRequest, "flow name is required")
		return
	}

	content, err := s.registry.Get(flowName)
	if err != nil {
		if errors.Is(err, registry.ErrFlowNotFound) {
			s.writeError(w, http.StatusNotFound, "flow not found")
			return
		}
		if errors.Is(err, registry.ErrInvalidFlowName) {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		log.Error().Err(err).Msg("Failed to get flow")
		s.writeError(w, http.StatusInternalServerError, "failed to get flow")
		return
	}

	s.writeJSON(w, http.StatusOK, FlowResponse{Name: flowName, Content: string(content)})
}

type AddFlowRequest struct {
	Content string `json:"content"`
}

type RenameFlowRequest struct {
	NewName string `json:"new_name"`
}

func (s *Server) handleAddFlow(w http.ResponseWriter, r *http.Request) {
	flowName := r.PathValue("name")
	if flowName == "" {
		s.writeError(w, http.StatusBadRequest, "flow name is required")
		return
	}

	var req AddFlowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Content == "" {
		s.writeError(w, http.StatusBadRequest, "content is required")
		return
	}

	if err := s.registry.Add(flowName, []byte(req.Content)); err != nil {
		if errors.Is(err, registry.ErrInvalidFlowName) {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		log.Error().Err(err).Msg("Failed to add flow")
		s.writeError(w, http.StatusInternalServerError, "failed to add flow")
		return
	}

	log.Info().Str("flow", flowName).Msg("Flow added")
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) handleDeleteFlow(w http.ResponseWriter, r *http.Request) {
	flowName := r.PathValue("name")
	if flowName == "" {
		s.writeError(w, http.StatusBadRequest, "flow name is required")
		return
	}

	if err := s.registry.Delete(flowName); err != nil {
		if errors.Is(err, registry.ErrFlowNotFound) {
			s.writeError(w, http.StatusNotFound, "flow not found")
			return
		}
		if errors.Is(err, registry.ErrInvalidFlowName) {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		log.Error().Err(err).Msg("Failed to delete flow")
		s.writeError(w, http.StatusInternalServerError, "failed to delete flow")
		return
	}

	log.Info().Str("flow", flowName).Msg("Flow deleted")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRenameFlow(w http.ResponseWriter, r *http.Request) {
	oldName := r.PathValue("name")
	if oldName == "" {
		s.writeError(w, http.StatusBadRequest, "flow name is required")
		return
	}

	var req RenameFlowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.NewName == "" {
		s.writeError(w, http.StatusBadRequest, "new_name is required")
		return
	}

	if err := s.registry.Rename(oldName, req.NewName); err != nil {
		if errors.Is(err, registry.ErrFlowNotFound) {
			s.writeError(w, http.StatusNotFound, "flow not found")
			return
		}
		if errors.Is(err, registry.ErrFlowExists) {
			s.writeError(w, http.StatusConflict, "flow already exists")
			return
		}
		if errors.Is(err, registry.ErrInvalidFlowName) {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		log.Error().Err(err).Msg("Failed to rename flow")
		s.writeError(w, http.StatusInternalServerError, "failed to rename flow")
		return
	}

	// Update schedule references if scheduler is configured
	if s.scheduler != nil {
		if err := s.scheduler.UpdateFlowReferences(oldName, req.NewName); err != nil {
			log.Warn().Err(err).Msg("Failed to update schedule references after flow rename")
			// Don't fail the request - flow rename succeeded
		}
	}

	log.Info().
		Str("old_name", oldName).
		Str("new_name", req.NewName).
		Msg("Flow renamed")

	w.WriteHeader(http.StatusNoContent)
}

// Schedule request/response types

type CreateScheduleRequest struct {
	Name       string            `json:"name"`
	Flow       string            `json:"flow"`
	Type       string            `json:"type"`
	Expression string            `json:"expression"`
	Inputs     map[string]string `json:"inputs,omitempty"`
	Enabled    bool              `json:"enabled"`
}

type UpdateScheduleRequest struct {
	Flow       string            `json:"flow"`
	Type       string            `json:"type"`
	Expression string            `json:"expression"`
	Inputs     map[string]string `json:"inputs,omitempty"`
	Enabled    bool              `json:"enabled"`
}

type ScheduleResponse struct {
	Name       string            `json:"name"`
	Flow       string            `json:"flow"`
	Type       string            `json:"type"`
	Expression string            `json:"expression"`
	Inputs     map[string]string `json:"inputs,omitempty"`
	Enabled    bool              `json:"enabled"`
	CreatedAt  time.Time         `json:"created_at"`
	LastRunAt  *time.Time        `json:"last_run_at,omitempty"`
	NextRunAt  *time.Time        `json:"next_run_at,omitempty"`
}

func scheduleToResponse(s *scheduler.ScheduleWithNextRun) ScheduleResponse {
	return ScheduleResponse{
		Name:       s.Name,
		Flow:       s.Flow,
		Type:       string(s.Type),
		Expression: s.Expression,
		Inputs:     s.Inputs,
		Enabled:    s.Enabled,
		CreatedAt:  s.CreatedAt,
		LastRunAt:  s.LastRunAt,
		NextRunAt:  s.NextRunAt,
	}
}

func (s *Server) handleListSchedules(w http.ResponseWriter, _ *http.Request) {
	if s.scheduler == nil {
		s.writeError(w, http.StatusServiceUnavailable, "scheduler not configured")
		return
	}

	schedules, err := s.scheduler.List()
	if err != nil {
		log.Error().Err(err).Msg("Failed to list schedules")
		s.writeError(w, http.StatusInternalServerError, "failed to list schedules")
		return
	}

	response := make([]ScheduleResponse, 0, len(schedules))
	for _, sched := range schedules {
		response = append(response, scheduleToResponse(sched))
	}

	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleGetSchedule(w http.ResponseWriter, r *http.Request) {
	if s.scheduler == nil {
		s.writeError(w, http.StatusServiceUnavailable, "scheduler not configured")
		return
	}

	scheduleName := r.PathValue("name")
	if scheduleName == "" {
		s.writeError(w, http.StatusBadRequest, "schedule name is required")
		return
	}

	sched, err := s.scheduler.Get(scheduleName)
	if err != nil {
		if errors.Is(err, scheduler.ErrScheduleNotFound) {
			s.writeError(w, http.StatusNotFound, "schedule not found")
			return
		}
		if errors.Is(err, scheduler.ErrInvalidScheduleName) {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		log.Error().Err(err).Msg("Failed to get schedule")
		s.writeError(w, http.StatusInternalServerError, "failed to get schedule")
		return
	}

	s.writeJSON(w, http.StatusOK, scheduleToResponse(sched))
}

func (s *Server) handleCreateSchedule(w http.ResponseWriter, r *http.Request) {
	if s.scheduler == nil {
		s.writeError(w, http.StatusServiceUnavailable, "scheduler not configured")
		return
	}

	var req CreateScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		s.writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Flow == "" {
		s.writeError(w, http.StatusBadRequest, "flow is required")
		return
	}
	if req.Type == "" {
		s.writeError(w, http.StatusBadRequest, "type is required")
		return
	}
	if req.Expression == "" {
		s.writeError(w, http.StatusBadRequest, "expression is required")
		return
	}

	sched := &scheduler.Schedule{
		Name:       req.Name,
		Flow:       req.Flow,
		Type:       scheduler.ScheduleType(req.Type),
		Expression: req.Expression,
		Inputs:     req.Inputs,
		Enabled:    req.Enabled,
	}

	if err := s.scheduler.Create(sched); err != nil {
		if errors.Is(err, scheduler.ErrScheduleExists) {
			s.writeError(w, http.StatusConflict, "schedule already exists")
			return
		}
		if errors.Is(err, scheduler.ErrInvalidScheduleName) {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, scheduler.ErrInvalidScheduleType) {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, scheduler.ErrInvalidExpression) {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, scheduler.ErrFlowNotFound) {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		log.Error().Err(err).Msg("Failed to create schedule")
		s.writeError(w, http.StatusInternalServerError, "failed to create schedule")
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (s *Server) handleUpdateSchedule(w http.ResponseWriter, r *http.Request) {
	if s.scheduler == nil {
		s.writeError(w, http.StatusServiceUnavailable, "scheduler not configured")
		return
	}

	scheduleName := r.PathValue("name")
	if scheduleName == "" {
		s.writeError(w, http.StatusBadRequest, "schedule name is required")
		return
	}

	var req UpdateScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Flow == "" {
		s.writeError(w, http.StatusBadRequest, "flow is required")
		return
	}
	if req.Type == "" {
		s.writeError(w, http.StatusBadRequest, "type is required")
		return
	}
	if req.Expression == "" {
		s.writeError(w, http.StatusBadRequest, "expression is required")
		return
	}

	sched := &scheduler.Schedule{
		Name:       scheduleName,
		Flow:       req.Flow,
		Type:       scheduler.ScheduleType(req.Type),
		Expression: req.Expression,
		Inputs:     req.Inputs,
		Enabled:    req.Enabled,
	}

	if err := s.scheduler.Update(sched); err != nil {
		if errors.Is(err, scheduler.ErrScheduleNotFound) {
			s.writeError(w, http.StatusNotFound, "schedule not found")
			return
		}
		if errors.Is(err, scheduler.ErrInvalidScheduleName) {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, scheduler.ErrInvalidScheduleType) {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, scheduler.ErrInvalidExpression) {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, scheduler.ErrFlowNotFound) {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		log.Error().Err(err).Msg("Failed to update schedule")
		s.writeError(w, http.StatusInternalServerError, "failed to update schedule")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteSchedule(w http.ResponseWriter, r *http.Request) {
	if s.scheduler == nil {
		s.writeError(w, http.StatusServiceUnavailable, "scheduler not configured")
		return
	}

	scheduleName := r.PathValue("name")
	if scheduleName == "" {
		s.writeError(w, http.StatusBadRequest, "schedule name is required")
		return
	}

	if err := s.scheduler.Delete(scheduleName); err != nil {
		if errors.Is(err, scheduler.ErrScheduleNotFound) {
			s.writeError(w, http.StatusNotFound, "schedule not found")
			return
		}
		if errors.Is(err, scheduler.ErrInvalidScheduleName) {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		log.Error().Err(err).Msg("Failed to delete schedule")
		s.writeError(w, http.StatusInternalServerError, "failed to delete schedule")
		return
	}

	log.Info().Str("schedule", scheduleName).Msg("Schedule deleted")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleEnableSchedule(w http.ResponseWriter, r *http.Request) {
	if s.scheduler == nil {
		s.writeError(w, http.StatusServiceUnavailable, "scheduler not configured")
		return
	}

	scheduleName := r.PathValue("name")
	if scheduleName == "" {
		s.writeError(w, http.StatusBadRequest, "schedule name is required")
		return
	}

	if err := s.scheduler.Enable(scheduleName); err != nil {
		if errors.Is(err, scheduler.ErrScheduleNotFound) {
			s.writeError(w, http.StatusNotFound, "schedule not found")
			return
		}
		if errors.Is(err, scheduler.ErrInvalidScheduleName) {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		log.Error().Err(err).Msg("Failed to enable schedule")
		s.writeError(w, http.StatusInternalServerError, "failed to enable schedule")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDisableSchedule(w http.ResponseWriter, r *http.Request) {
	if s.scheduler == nil {
		s.writeError(w, http.StatusServiceUnavailable, "scheduler not configured")
		return
	}

	scheduleName := r.PathValue("name")
	if scheduleName == "" {
		s.writeError(w, http.StatusBadRequest, "schedule name is required")
		return
	}

	if err := s.scheduler.Disable(scheduleName); err != nil {
		if errors.Is(err, scheduler.ErrScheduleNotFound) {
			s.writeError(w, http.StatusNotFound, "schedule not found")
			return
		}
		if errors.Is(err, scheduler.ErrInvalidScheduleName) {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		log.Error().Err(err).Msg("Failed to disable schedule")
		s.writeError(w, http.StatusInternalServerError, "failed to disable schedule")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ProfileResponse represents the authenticated user's profile.
type ProfileResponse struct {
	Username string   `json:"username"`
	Roles    []string `json:"roles"`
}

func (s *Server) handleProfile(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())

	if user == nil {
		// No user in context means auth is not required for this transport.
		// Return empty profile to indicate no authentication needed.
		s.writeJSON(w, http.StatusOK, ProfileResponse{
			Username: "",
			Roles:    []string{},
		})
		return
	}

	// Convert roles to strings
	roles := make([]string, len(user.Roles))
	for i, role := range user.Roles {
		roles[i] = string(role)
	}

	s.writeJSON(w, http.StatusOK, ProfileResponse{
		Username: user.Name,
		Roles:    roles,
	})
}
