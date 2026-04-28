package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"git.myservermanager.com/varakh/yafe/internal/auth"
	"git.myservermanager.com/varakh/yafe/internal/engine"
	"git.myservermanager.com/varakh/yafe/internal/queue"
	"git.myservermanager.com/varakh/yafe/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testAuthenticator is a simple authenticator for testing.
type testAuthenticator struct {
	users map[string]*auth.User
}

func (a *testAuthenticator) Authenticate(key string) *auth.User {
	return a.users[key]
}

// setupTestServer creates a server with real queue and registry on temp dirs.
func setupTestServer(t *testing.T) *Server {
	t.Helper()
	tmpDir := t.TempDir()

	flowsDir := filepath.Join(tmpDir, "flows")
	queueDir := filepath.Join(tmpDir, "queue")

	reg, err := registry.NewFileFlowRegistry(flowsDir)
	require.NoError(t, err)

	q, err := queue.NewFileQueue(queueDir, reg, queue.CleanupConfig{})
	require.NoError(t, err)

	srv := New(q, reg, nil, Config{
		SocketPath: filepath.Join(tmpDir, "yafe.sock"),
		SocketAuth: false,
		HTTPAuth:   false,
	}, nil)

	return srv
}

// setupTestServerWithAuth creates a server with authentication enabled.
func setupTestServerWithAuth(t *testing.T) (*Server, *testAuthenticator) {
	t.Helper()
	tmpDir := t.TempDir()

	flowsDir := filepath.Join(tmpDir, "flows")
	queueDir := filepath.Join(tmpDir, "queue")

	reg, err := registry.NewFileFlowRegistry(flowsDir)
	require.NoError(t, err)

	q, err := queue.NewFileQueue(queueDir, reg, queue.CleanupConfig{})
	require.NoError(t, err)

	authenticator := &testAuthenticator{
		users: map[string]*auth.User{
			"admin-key": {
				Name:  "admin",
				Roles: []auth.Role{auth.RoleJobsread, auth.RoleJobswrite, auth.RoleFlowsread, auth.RoleFlowswrite},
			},
			"reader-key": {
				Name:  "reader",
				Roles: []auth.Role{auth.RoleJobsread, auth.RoleFlowsread},
			},
		},
	}

	srv := New(q, reg, nil, Config{
		SocketPath: filepath.Join(tmpDir, "yafe.sock"),
		SocketAuth: true,
		HTTPAuth:   true,
		Auth:       authenticator,
	}, nil)

	return srv, authenticator
}

func addFlow(t *testing.T, srv *Server, name, content string) {
	t.Helper()
	body := `{"content": "` + strings.ReplaceAll(content, "\n", "\\n") + `"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/flows/"+name, strings.NewReader(body))
	req.Header.Set("Content-Type", contentTypeJSON)
	rec := httptest.NewRecorder()
	srv.socketHandler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, "failed to add flow: %s", rec.Body.String())
}

func TestServerIntegration_Flows(t *testing.T) {
	t.Run("add flow", func(t *testing.T) {
		srv := setupTestServer(t)
		content := "runs-on: host\nsteps:\n  - cmd: echo test\n    kind: shell"
		body := `{"content": "` + strings.ReplaceAll(content, "\n", "\\n") + `"}`

		req := httptest.NewRequest(http.MethodPut, "/api/v1/flows/myflow", strings.NewReader(body))
		req.Header.Set("Content-Type", contentTypeJSON)
		rec := httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)
	})

	t.Run("add flow with invalid name returns 400", func(t *testing.T) {
		srv := setupTestServer(t)
		body := `{"content": "test"}`

		req := httptest.NewRequest(http.MethodPut, "/api/v1/flows/invalid.name", strings.NewReader(body))
		req.Header.Set("Content-Type", contentTypeJSON)
		rec := httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assertJSONError(t, rec, "invalid flow name")
	})

	t.Run("add flow without content returns 400", func(t *testing.T) {
		srv := setupTestServer(t)
		body := `{"content": ""}`

		req := httptest.NewRequest(http.MethodPut, "/api/v1/flows/myflow", strings.NewReader(body))
		req.Header.Set("Content-Type", contentTypeJSON)
		rec := httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assertJSONError(t, rec, "content is required")
	})

	t.Run("list flows", func(t *testing.T) {
		srv := setupTestServer(t)
		addFlow(t, srv, "flow1", "runs-on: host\nsteps:\n  - cmd: echo 1\n    kind: shell")
		addFlow(t, srv, "flow2", "runs-on: host\nsteps:\n  - cmd: echo 2\n    kind: shell")

		req := httptest.NewRequest(http.MethodGet, "/api/v1/flows", nil)
		rec := httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var flows []string
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &flows))
		assert.ElementsMatch(t, []string{"flow1", "flow2"}, flows)
	})

	t.Run("list flows returns empty array when none exist", func(t *testing.T) {
		srv := setupTestServer(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/flows", nil)
		rec := httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var flows []string
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &flows))
		assert.Empty(t, flows)
	})

	t.Run("get flow", func(t *testing.T) {
		srv := setupTestServer(t)
		content := "runs-on: host\nsteps:\n  - cmd: echo test\n    kind: shell"
		addFlow(t, srv, "myflow", content)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/flows/myflow", nil)
		rec := httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp FlowResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, "myflow", resp.Name)
		assert.Equal(t, content, resp.Content)
	})

	t.Run("get nonexistent flow returns 404", func(t *testing.T) {
		srv := setupTestServer(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/flows/nonexistent", nil)
		rec := httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
		assertJSONError(t, rec, "flow not found")
	})

	t.Run("get flow with invalid name returns 400", func(t *testing.T) {
		srv := setupTestServer(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/flows/invalid.name", nil)
		rec := httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("delete flow", func(t *testing.T) {
		srv := setupTestServer(t)
		addFlow(t, srv, "todelete", "runs-on: host\nsteps:\n  - cmd: echo\n    kind: shell")

		req := httptest.NewRequest(http.MethodDelete, "/api/v1/flows/todelete", nil)
		rec := httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNoContent, rec.Code)

		// Verify flow is deleted
		req = httptest.NewRequest(http.MethodGet, "/api/v1/flows/todelete", nil)
		rec = httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("delete nonexistent flow returns 404", func(t *testing.T) {
		srv := setupTestServer(t)

		req := httptest.NewRequest(http.MethodDelete, "/api/v1/flows/nonexistent", nil)
		rec := httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestServerIntegration_Jobs(t *testing.T) {
	t.Run("enqueue job", func(t *testing.T) {
		srv := setupTestServer(t)
		addFlow(t, srv, "myflow", "runs-on: host\nsteps:\n  - cmd: echo test\n    kind: shell")

		body := `{"flow": "myflow"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", strings.NewReader(body))
		req.Header.Set("Content-Type", contentTypeJSON)
		rec := httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusAccepted, rec.Code)

		var resp EnqueueResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.NotEmpty(t, resp.JobID)
	})

	t.Run("enqueue job with inputs", func(t *testing.T) {
		srv := setupTestServer(t)
		addFlow(t, srv, "myflow", "runs-on: host\nsteps:\n  - cmd: echo test\n    kind: shell")

		body := `{"flow": "myflow", "inputs": {"key1": "value1", "key2": "value2"}}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", strings.NewReader(body))
		req.Header.Set("Content-Type", contentTypeJSON)
		rec := httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusAccepted, rec.Code)
	})

	t.Run("enqueue job for nonexistent flow returns 404", func(t *testing.T) {
		srv := setupTestServer(t)

		body := `{"flow": "nonexistent"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", strings.NewReader(body))
		req.Header.Set("Content-Type", contentTypeJSON)
		rec := httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("enqueue job without flow returns 400", func(t *testing.T) {
		srv := setupTestServer(t)

		body := `{}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", strings.NewReader(body))
		req.Header.Set("Content-Type", contentTypeJSON)
		rec := httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assertJSONError(t, rec, "flow is required")
	})

	t.Run("enqueue job with invalid body returns 400", func(t *testing.T) {
		srv := setupTestServer(t)

		body := `not valid json`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", strings.NewReader(body))
		req.Header.Set("Content-Type", contentTypeJSON)
		rec := httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assertJSONError(t, rec, "invalid request body")
	})

	t.Run("list jobs", func(t *testing.T) {
		srv := setupTestServer(t)
		addFlow(t, srv, "myflow", "runs-on: host\nsteps:\n  - cmd: echo test\n    kind: shell")

		// Enqueue some jobs
		for i := 0; i < 3; i++ {
			body := `{"flow": "myflow"}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", strings.NewReader(body))
			req.Header.Set("Content-Type", contentTypeJSON)
			rec := httptest.NewRecorder()
			srv.socketHandler.ServeHTTP(rec, req)
			require.Equal(t, http.StatusAccepted, rec.Code)
		}

		req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs", nil)
		rec := httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var jobs []jobWithStatus
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &jobs))
		assert.Len(t, jobs, 3)
	})

	t.Run("list jobs with status filter", func(t *testing.T) {
		srv := setupTestServer(t)
		addFlow(t, srv, "myflow", "runs-on: host\nsteps:\n  - cmd: echo test\n    kind: shell")

		// Enqueue a job
		body := `{"flow": "myflow"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", strings.NewReader(body))
		req.Header.Set("Content-Type", contentTypeJSON)
		rec := httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusAccepted, rec.Code)

		// List only pending jobs
		req = httptest.NewRequest(http.MethodGet, "/api/v1/jobs?status=pending", nil)
		rec = httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var jobs []jobWithStatus
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &jobs))
		assert.Len(t, jobs, 1)
		assert.Equal(t, queue.JobStatusPending, jobs[0].Status)
	})

	t.Run("list jobs with invalid status returns 400", func(t *testing.T) {
		srv := setupTestServer(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs?status=invalid", nil)
		rec := httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("list jobs returns empty array when none exist", func(t *testing.T) {
		srv := setupTestServer(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs", nil)
		rec := httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var jobs []jobWithStatus
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &jobs))
		assert.Empty(t, jobs)
	})

	t.Run("get job", func(t *testing.T) {
		srv := setupTestServer(t)
		addFlow(t, srv, "myflow", "runs-on: host\nsteps:\n  - cmd: echo test\n    kind: shell")

		// Enqueue a job
		body := `{"flow": "myflow"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", strings.NewReader(body))
		req.Header.Set("Content-Type", contentTypeJSON)
		rec := httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusAccepted, rec.Code)

		var enqueueResp EnqueueResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &enqueueResp))

		// Get the job
		req = httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+enqueueResp.JobID, nil)
		rec = httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var jobResp struct {
			ID     string          `json:"id"`
			Flow   string          `json:"flow"`
			Status queue.JobStatus `json:"status"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &jobResp))
		assert.Equal(t, enqueueResp.JobID, jobResp.ID)
		assert.Equal(t, "myflow", jobResp.Flow)
		assert.Equal(t, queue.JobStatusPending, jobResp.Status)
	})

	t.Run("get nonexistent job returns 404", func(t *testing.T) {
		srv := setupTestServer(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/nonexistent", nil)
		rec := httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
		assertJSONError(t, rec, "job not found")
	})

	t.Run("delete job", func(t *testing.T) {
		srv := setupTestServer(t)
		addFlow(t, srv, "myflow", "runs-on: host\nsteps:\n  - cmd: echo test\n    kind: shell")

		// Enqueue a job
		body := `{"flow": "myflow"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", strings.NewReader(body))
		req.Header.Set("Content-Type", contentTypeJSON)
		rec := httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)

		var enqueueResp EnqueueResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &enqueueResp))

		// Delete the job
		req = httptest.NewRequest(http.MethodDelete, "/api/v1/jobs/"+enqueueResp.JobID, nil)
		rec = httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNoContent, rec.Code)

		// Verify job is deleted
		req = httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+enqueueResp.JobID, nil)
		rec = httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("delete nonexistent job returns 404", func(t *testing.T) {
		srv := setupTestServer(t)

		req := httptest.NewRequest(http.MethodDelete, "/api/v1/jobs/nonexistent", nil)
		rec := httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestServerIntegration_Auth(t *testing.T) {
	t.Run("unauthenticated request returns 401", func(t *testing.T) {
		srv, _ := setupTestServerWithAuth(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs", nil)
		rec := httptest.NewRecorder()
		srv.httpHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assertJSONError(t, rec, "authentication required")
	})

	t.Run("invalid API key returns 401", func(t *testing.T) {
		srv, _ := setupTestServerWithAuth(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs", nil)
		req.Header.Set(auth.HeaderAPIKey, "invalid-key")
		rec := httptest.NewRecorder()
		srv.httpHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assertJSONError(t, rec, "invalid API key")
	})

	t.Run("valid API key with sufficient role succeeds", func(t *testing.T) {
		srv, _ := setupTestServerWithAuth(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs", nil)
		req.Header.Set(auth.HeaderAPIKey, "admin-key")
		rec := httptest.NewRecorder()
		srv.httpHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("valid API key without required role returns 403", func(t *testing.T) {
		srv, _ := setupTestServerWithAuth(t)

		// reader-key has only read permissions, not write
		body := `{"flow": "test"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", strings.NewReader(body))
		req.Header.Set(auth.HeaderAPIKey, "reader-key")
		req.Header.Set("Content-Type", contentTypeJSON)
		rec := httptest.NewRecorder()
		srv.httpHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusForbidden, rec.Code)
		assertJSONError(t, rec, "insufficient permissions")
	})

	t.Run("reader can read jobs", func(t *testing.T) {
		srv, _ := setupTestServerWithAuth(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs", nil)
		req.Header.Set(auth.HeaderAPIKey, "reader-key")
		rec := httptest.NewRecorder()
		srv.httpHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("reader can read flows", func(t *testing.T) {
		srv, _ := setupTestServerWithAuth(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/flows", nil)
		req.Header.Set(auth.HeaderAPIKey, "reader-key")
		rec := httptest.NewRecorder()
		srv.httpHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("reader cannot write flows", func(t *testing.T) {
		srv, _ := setupTestServerWithAuth(t)

		body := `{"content": "test"}`
		req := httptest.NewRequest(http.MethodPut, "/api/v1/flows/test", strings.NewReader(body))
		req.Header.Set(auth.HeaderAPIKey, "reader-key")
		req.Header.Set("Content-Type", contentTypeJSON)
		rec := httptest.NewRecorder()
		srv.httpHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("reader cannot delete flows", func(t *testing.T) {
		srv, _ := setupTestServerWithAuth(t)

		req := httptest.NewRequest(http.MethodDelete, "/api/v1/flows/test", nil)
		req.Header.Set(auth.HeaderAPIKey, "reader-key")
		rec := httptest.NewRecorder()
		srv.httpHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("admin can perform all operations", func(t *testing.T) {
		srv, _ := setupTestServerWithAuth(t)

		// Add flow
		body := `{"content": "runs-on: host\nsteps:\n  - cmd: echo test\n    kind: shell"}`
		req := httptest.NewRequest(http.MethodPut, "/api/v1/flows/testflow", strings.NewReader(body))
		req.Header.Set(auth.HeaderAPIKey, "admin-key")
		req.Header.Set("Content-Type", contentTypeJSON)
		rec := httptest.NewRecorder()
		srv.httpHandler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusCreated, rec.Code)

		// Enqueue job
		body = `{"flow": "testflow"}`
		req = httptest.NewRequest(http.MethodPost, "/api/v1/jobs", strings.NewReader(body))
		req.Header.Set(auth.HeaderAPIKey, "admin-key")
		req.Header.Set("Content-Type", contentTypeJSON)
		rec = httptest.NewRecorder()
		srv.httpHandler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusAccepted, rec.Code)

		var enqueueResp EnqueueResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &enqueueResp))

		// Delete job
		req = httptest.NewRequest(http.MethodDelete, "/api/v1/jobs/"+enqueueResp.JobID, nil)
		req.Header.Set(auth.HeaderAPIKey, "admin-key")
		rec = httptest.NewRecorder()
		srv.httpHandler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNoContent, rec.Code)

		// Delete flow
		req = httptest.NewRequest(http.MethodDelete, "/api/v1/flows/testflow", nil)
		req.Header.Set(auth.HeaderAPIKey, "admin-key")
		rec = httptest.NewRecorder()
		srv.httpHandler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})
}

func TestServerIntegration_FullLifecycle(t *testing.T) {
	t.Run("full job lifecycle: add flow -> enqueue -> get -> delete", func(t *testing.T) {
		srv := setupTestServer(t)

		// 1. Add a flow
		flowContent := "runs-on: host\nsteps:\n  - cmd: echo hello\n    kind: shell"
		body := `{"content": "` + strings.ReplaceAll(flowContent, "\n", "\\n") + `"}`
		req := httptest.NewRequest(http.MethodPut, "/api/v1/flows/hello-flow", strings.NewReader(body))
		req.Header.Set("Content-Type", contentTypeJSON)
		rec := httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusCreated, rec.Code)

		// 2. Verify flow is listed
		req = httptest.NewRequest(http.MethodGet, "/api/v1/flows", nil)
		rec = httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var flows []string
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &flows))
		assert.Contains(t, flows, "hello-flow")

		// 3. Enqueue a job
		body = `{"flow": "hello-flow", "inputs": {"name": "world"}}`
		req = httptest.NewRequest(http.MethodPost, "/api/v1/jobs", strings.NewReader(body))
		req.Header.Set("Content-Type", contentTypeJSON)
		rec = httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusAccepted, rec.Code)

		var enqueueResp EnqueueResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &enqueueResp))

		// 4. Get the job and verify details
		req = httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+enqueueResp.JobID, nil)
		rec = httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var jobResp struct {
			ID     string            `json:"id"`
			Flow   string            `json:"flow"`
			Inputs map[string]string `json:"inputs"`
			Status queue.JobStatus   `json:"status"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &jobResp))
		assert.Equal(t, enqueueResp.JobID, jobResp.ID)
		assert.Equal(t, "hello-flow", jobResp.Flow)
		assert.Equal(t, "world", jobResp.Inputs["name"])
		assert.Equal(t, queue.JobStatusPending, jobResp.Status)

		// 5. List jobs and verify the job appears
		req = httptest.NewRequest(http.MethodGet, "/api/v1/jobs", nil)
		rec = httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var jobs []jobWithStatus
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &jobs))
		assert.Len(t, jobs, 1)
		assert.Equal(t, enqueueResp.JobID, jobs[0].ID)

		// 6. Delete the job
		req = httptest.NewRequest(http.MethodDelete, "/api/v1/jobs/"+enqueueResp.JobID, nil)
		rec = httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusNoContent, rec.Code)

		// 7. Verify job is gone
		req = httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+enqueueResp.JobID, nil)
		rec = httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code)

		// 8. Delete the flow
		req = httptest.NewRequest(http.MethodDelete, "/api/v1/flows/hello-flow", nil)
		rec = httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusNoContent, rec.Code)

		// 9. Verify flow is gone
		req = httptest.NewRequest(http.MethodGet, "/api/v1/flows/hello-flow", nil)
		rec = httptest.NewRecorder()
		srv.socketHandler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func assertJSONError(t *testing.T, rec *httptest.ResponseRecorder, expectedMsg string) {
	t.Helper()
	var resp ErrorResponse
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err, "response body: %s", rec.Body.String())
	assert.Contains(t, resp.Error, expectedMsg)
}

// testEnvironment holds the full test setup for end-to-end tests.
type testEnvironment struct {
	server   *Server
	queue    *queue.FileQueue
	registry *registry.FileFlowRegistry
	engine   *engine.Engine
	worker   *queue.Worker
	cancel   context.CancelFunc
	tmpDir   string
}

// setupE2EEnvironment creates a full test environment with server and worker.
func setupE2EEnvironment(t *testing.T) *testEnvironment {
	t.Helper()
	tmpDir := t.TempDir()

	flowsDir := filepath.Join(tmpDir, "flows")
	queueDir := filepath.Join(tmpDir, "queue")

	reg, err := registry.NewFileFlowRegistry(flowsDir)
	require.NoError(t, err)

	q, err := queue.NewFileQueue(queueDir, reg, queue.CleanupConfig{})
	require.NoError(t, err)

	e := engine.NewEngine()

	workerConfig := queue.WorkerConfig{
		PollInterval:    50 * time.Millisecond,
		CleanupInterval: time.Hour,
	}
	w := queue.NewWorker(q, reg, e, workerConfig)

	srv := New(q, reg, nil, Config{
		SocketPath: filepath.Join(tmpDir, "yafe.sock"),
		SocketAuth: false,
		HTTPAuth:   false,
	}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	go w.Run(ctx)

	return &testEnvironment{
		server:   srv,
		queue:    q,
		registry: reg,
		engine:   e,
		worker:   w,
		cancel:   cancel,
		tmpDir:   tmpDir,
	}
}

func (env *testEnvironment) cleanup() {
	env.cancel()
	<-env.worker.Done()
}

func (env *testEnvironment) addFlowFromFile(t *testing.T, name, filePath string) {
	t.Helper()
	content, err := os.ReadFile(filePath)
	require.NoError(t, err, "failed to read example file: %s", filePath)

	body := `{"content": "` + strings.ReplaceAll(strings.ReplaceAll(string(content), "\\", "\\\\"), "\n", "\\n") + `"}`
	body = strings.ReplaceAll(body, "\t", "\\t")
	body = strings.ReplaceAll(body, "\"", "\\\"")

	// Re-encode properly using JSON
	bodyMap := map[string]string{"content": string(content)}
	bodyBytes, err := json.Marshal(bodyMap)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/flows/"+name, strings.NewReader(string(bodyBytes)))
	req.Header.Set("Content-Type", contentTypeJSON)
	rec := httptest.NewRecorder()
	env.server.socketHandler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, "failed to add flow %s: %s", name, rec.Body.String())
}

func (env *testEnvironment) addFlow(t *testing.T, name, content string) {
	t.Helper()
	bodyMap := map[string]string{"content": content}
	bodyBytes, err := json.Marshal(bodyMap)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/flows/"+name, strings.NewReader(string(bodyBytes)))
	req.Header.Set("Content-Type", contentTypeJSON)
	rec := httptest.NewRecorder()
	env.server.socketHandler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, "failed to add flow %s: %s", name, rec.Body.String())
}

func (env *testEnvironment) enqueueJob(t *testing.T, flowName string, inputs map[string]string) string {
	t.Helper()
	body := map[string]any{"flow": flowName}
	if inputs != nil {
		body["inputs"] = inputs
	}
	bodyBytes, err := json.Marshal(body)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", strings.NewReader(string(bodyBytes)))
	req.Header.Set("Content-Type", contentTypeJSON)
	rec := httptest.NewRecorder()
	env.server.socketHandler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code, "failed to enqueue job: %s", rec.Body.String())

	var resp EnqueueResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp.JobID
}

func (env *testEnvironment) waitForJob(t *testing.T, jobID string, timeout time.Duration) queue.JobStatus {
	t.Helper()
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+jobID, nil)
		rec := httptest.NewRecorder()
		env.server.socketHandler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var jobResp struct {
			Status queue.JobStatus `json:"status"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &jobResp))

		if jobResp.Status == queue.JobStatusDone || jobResp.Status == queue.JobStatusFailed {
			return jobResp.Status
		}

		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("job %s did not complete within %v", jobID, timeout)
	return ""
}

func (env *testEnvironment) getJobLogs(t *testing.T, jobID string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+jobID+"/logs", nil)
	rec := httptest.NewRecorder()
	env.server.socketHandler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "failed to get logs: %s", rec.Body.String())
	return rec.Body.String()
}

// getExamplesDir returns the path to _doc/examples directory.
func getExamplesDir(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok, "failed to get caller info")

	// Navigate from internal/server/server_test.go to _doc/examples
	serverDir := filepath.Dir(filename)
	projectRoot := filepath.Join(serverDir, "..", "..")
	examplesDir := filepath.Join(projectRoot, "_doc", "examples")

	_, err := os.Stat(examplesDir)
	require.NoError(t, err, "examples directory not found: %s", examplesDir)

	return examplesDir
}

func TestE2E_Examples(t *testing.T) {
	examplesDir := getExamplesDir(t)

	t.Run("01_minimal", func(t *testing.T) {
		env := setupE2EEnvironment(t)
		defer env.cleanup()

		env.addFlowFromFile(t, "minimal", filepath.Join(examplesDir, "01_minimal.yaml"))
		jobID := env.enqueueJob(t, "minimal", nil)
		status := env.waitForJob(t, jobID, 10*time.Second)

		assert.Equal(t, queue.JobStatusDone, status)
		logs := env.getJobLogs(t, jobID)
		assert.Contains(t, logs, "Hello, World!")
	})

	t.Run("02_shell_options", func(t *testing.T) {
		env := setupE2EEnvironment(t)
		defer env.cleanup()

		env.addFlowFromFile(t, "shell-options", filepath.Join(examplesDir, "02_shell_options.yaml"))
		jobID := env.enqueueJob(t, "shell-options", nil)
		status := env.waitForJob(t, jobID, 10*time.Second)

		assert.Equal(t, queue.JobStatusDone, status)
		logs := env.getJobLogs(t, jobID)
		assert.Contains(t, logs, "Using default shell")
		assert.Contains(t, logs, "Using bash explicitly")
		assert.Contains(t, logs, "Using sh")
	})

	t.Run("03_environment", func(t *testing.T) {
		env := setupE2EEnvironment(t)
		defer env.cleanup()

		env.addFlowFromFile(t, "environment", filepath.Join(examplesDir, "03_environment.yaml"))
		jobID := env.enqueueJob(t, "environment", nil)
		status := env.waitForJob(t, jobID, 10*time.Second)

		assert.Equal(t, queue.JobStatusDone, status)
		logs := env.getJobLogs(t, jobID)
		assert.Contains(t, logs, "DEBUG=true")
		assert.Contains(t, logs, "APP_NAME=myapp")
		assert.Contains(t, logs, "VERSION=1.0.0")
	})

	t.Run("04_multiline", func(t *testing.T) {
		env := setupE2EEnvironment(t)
		defer env.cleanup()

		env.addFlowFromFile(t, "multiline", filepath.Join(examplesDir, "04_multiline.yaml"))
		jobID := env.enqueueJob(t, "multiline", nil)
		status := env.waitForJob(t, jobID, 10*time.Second)

		assert.Equal(t, queue.JobStatusDone, status)
		logs := env.getJobLogs(t, jobID)
		assert.Contains(t, logs, "Line 1: Setting up")
		assert.Contains(t, logs, "Line 2: Processing")
		assert.Contains(t, logs, "Line 3: Complete")
	})

	t.Run("05_outputs_variable", func(t *testing.T) {
		env := setupE2EEnvironment(t)
		defer env.cleanup()

		env.addFlowFromFile(t, "outputs-var", filepath.Join(examplesDir, "05_outputs_variable.yaml"))
		jobID := env.enqueueJob(t, "outputs-var", nil)
		status := env.waitForJob(t, jobID, 10*time.Second)

		assert.Equal(t, queue.JobStatusDone, status)
		logs := env.getJobLogs(t, jobID)
		assert.Contains(t, logs, "Version: 1.2.3")
		assert.Contains(t, logs, "Build: 42")
	})

	t.Run("06_outputs_file", func(t *testing.T) {
		env := setupE2EEnvironment(t)
		defer env.cleanup()

		env.addFlowFromFile(t, "outputs-file", filepath.Join(examplesDir, "06_outputs_file.yaml"))
		jobID := env.enqueueJob(t, "outputs-file", nil)
		status := env.waitForJob(t, jobID, 10*time.Second)

		assert.Equal(t, queue.JobStatusDone, status)
		logs := env.getJobLogs(t, jobID)
		assert.Contains(t, logs, "Deploying artifact")
		assert.Contains(t, logs, "artifact content here")
	})

	t.Run("07_step_references", func(t *testing.T) {
		env := setupE2EEnvironment(t)
		defer env.cleanup()

		env.addFlowFromFile(t, "step-refs", filepath.Join(examplesDir, "07_step_references.yaml"))
		jobID := env.enqueueJob(t, "step-refs", nil)
		status := env.waitForJob(t, jobID, 10*time.Second)

		assert.Equal(t, queue.JobStatusDone, status)
		logs := env.getJobLogs(t, jobID)
		assert.Contains(t, logs, "Testing version 2.0.0")
		assert.Contains(t, logs, "Commit: abc1234")
		assert.Contains(t, logs, "Deploying 2.0.0")
		assert.Contains(t, logs, "compiled binary")
	})

	t.Run("08_secrets_env", func(t *testing.T) {
		env := setupE2EEnvironment(t)
		defer env.cleanup()

		// Set up the required environment variable
		t.Setenv("TEST_API_KEY", "secret-key-12345")

		env.addFlowFromFile(t, "secrets-env", filepath.Join(examplesDir, "08_secrets_env.yaml"))
		jobID := env.enqueueJob(t, "secrets-env", nil)
		status := env.waitForJob(t, jobID, 10*time.Second)

		assert.Equal(t, queue.JobStatusDone, status)
		logs := env.getJobLogs(t, jobID)
		assert.Contains(t, logs, "Using API key: secret-key-12345")
	})

	t.Run("09_secrets_file", func(t *testing.T) {
		env := setupE2EEnvironment(t)
		defer env.cleanup()

		// Create the secret file
		secretFile := "/tmp/yafe-test-secret.txt"
		err := os.WriteFile(secretFile, []byte("file-secret-password"), 0600)
		require.NoError(t, err)
		defer os.Remove(secretFile)

		env.addFlowFromFile(t, "secrets-file", filepath.Join(examplesDir, "09_secrets_file.yaml"))
		jobID := env.enqueueJob(t, "secrets-file", nil)
		status := env.waitForJob(t, jobID, 10*time.Second)

		assert.Equal(t, queue.JobStatusDone, status)
		logs := env.getJobLogs(t, jobID)
		assert.Contains(t, logs, "Connecting with password: file-secret-password")
	})

	t.Run("10_secrets_override", func(t *testing.T) {
		env := setupE2EEnvironment(t)
		defer env.cleanup()

		// Set up the required environment variables
		t.Setenv("FLOW_TOKEN", "flow-token-abc")
		t.Setenv("STEP_TOKEN", "step-token-xyz")

		env.addFlowFromFile(t, "secrets-override", filepath.Join(examplesDir, "10_secrets_override.yaml"))
		jobID := env.enqueueJob(t, "secrets-override", nil)
		status := env.waitForJob(t, jobID, 10*time.Second)

		assert.Equal(t, queue.JobStatusDone, status)
		logs := env.getJobLogs(t, jobID)
		assert.Contains(t, logs, "Flow token: flow-token-abc")
		assert.Contains(t, logs, "Step token (overridden): step-token-xyz")
	})

	t.Run("11_custom_state_dir", func(t *testing.T) {
		env := setupE2EEnvironment(t)
		defer env.cleanup()

		// Clean up custom state dir if it exists
		os.RemoveAll("/tmp/yafe-custom-state-test")
		defer os.RemoveAll("/tmp/yafe-custom-state-test")

		env.addFlowFromFile(t, "custom-state", filepath.Join(examplesDir, "11_custom_state_dir.yaml"))
		jobID := env.enqueueJob(t, "custom-state", nil)
		status := env.waitForJob(t, jobID, 10*time.Second)

		assert.Equal(t, queue.JobStatusDone, status)
		logs := env.getJobLogs(t, jobID)
		assert.Contains(t, logs, "/tmp/yafe-custom-state-test")
	})

	t.Run("12_job_inputs", func(t *testing.T) {
		env := setupE2EEnvironment(t)
		defer env.cleanup()

		env.addFlowFromFile(t, "job-inputs", filepath.Join(examplesDir, "12_job_inputs.yaml"))
		jobID := env.enqueueJob(t, "job-inputs", map[string]string{
			"environment": "production",
			"version":     "3.0.0",
		})
		status := env.waitForJob(t, jobID, 10*time.Second)

		assert.Equal(t, queue.JobStatusDone, status)
		logs := env.getJobLogs(t, jobID)
		assert.Contains(t, logs, "Environment: production")
		assert.Contains(t, logs, "Version: 3.0.0")
	})

	t.Run("13_tools", func(t *testing.T) {
		env := setupE2EEnvironment(t)
		defer env.cleanup()

		env.addFlowFromFile(t, "tools", filepath.Join(examplesDir, "13_tools.yaml"))
		jobID := env.enqueueJob(t, "tools", nil)
		status := env.waitForJob(t, jobID, 10*time.Second)

		assert.Equal(t, queue.JobStatusDone, status)
		logs := env.getJobLogs(t, jobID)
		assert.Contains(t, logs, "Tool 'bash' verified via PATH lookup")
	})

	t.Run("14_tools_download", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping network-dependent test in short mode")
		}

		env := setupE2EEnvironment(t)
		defer env.cleanup()

		env.addFlowFromFile(t, "tools-download", filepath.Join(examplesDir, "14_tools_download.yaml"))
		jobID := env.enqueueJob(t, "tools-download", nil)
		status := env.waitForJob(t, jobID, 60*time.Second) // longer timeout for download

		assert.Equal(t, queue.JobStatusDone, status)
		logs := env.getJobLogs(t, jobID)
		assert.Contains(t, logs, "hello world")
	})
}

func TestE2E_ToolDownload(t *testing.T) {
	t.Run("download and execute tool", func(t *testing.T) {
		// Set up a test HTTP server to serve the tool binary
		toolContent := "#!/usr/bin/env bash\necho \"downloaded-tool-output: $1\""
		toolServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(toolContent))
		}))
		defer toolServer.Close()

		env := setupE2EEnvironment(t)
		defer env.cleanup()

		// Create a flow that downloads the tool and uses it
		flowContent := `runs-on: host
tools:
  - name: mytool
    url: ` + toolServer.URL + `/mytool
steps:
  - cmd: mytool hello
    kind: shell
`
		env.addFlow(t, "tool-download", flowContent)
		jobID := env.enqueueJob(t, "tool-download", nil)
		status := env.waitForJob(t, jobID, 10*time.Second)

		assert.Equal(t, queue.JobStatusDone, status)
		logs := env.getJobLogs(t, jobID)
		assert.Contains(t, logs, "downloaded-tool-output: hello")
	})

	t.Run("download tool with checksum verification", func(t *testing.T) {
		toolContent := "#!/usr/bin/env bash\necho \"verified-tool\""

		// Compute SHA256 checksum
		h := sha256.Sum256([]byte(toolContent))
		checksum := hex.EncodeToString(h[:])

		toolServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(toolContent))
		}))
		defer toolServer.Close()

		env := setupE2EEnvironment(t)
		defer env.cleanup()

		flowContent := `runs-on: host
tools:
  - name: verified
    url: ` + toolServer.URL + `/tool
    sha256: ` + checksum + `
steps:
  - cmd: verified
    kind: shell
`
		env.addFlow(t, "tool-checksum", flowContent)
		jobID := env.enqueueJob(t, "tool-checksum", nil)
		status := env.waitForJob(t, jobID, 10*time.Second)

		assert.Equal(t, queue.JobStatusDone, status)
		logs := env.getJobLogs(t, jobID)
		assert.Contains(t, logs, "verified-tool")
	})

	t.Run("tool checksum mismatch fails", func(t *testing.T) {
		toolContent := "#!/usr/bin/env bash\necho \"bad-tool\""

		toolServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(toolContent))
		}))
		defer toolServer.Close()

		env := setupE2EEnvironment(t)
		defer env.cleanup()

		flowContent := `runs-on: host
tools:
  - name: badtool
    url: ` + toolServer.URL + `/tool
    sha256: "0000000000000000000000000000000000000000000000000000000000000000"
steps:
  - cmd: badtool
    kind: shell
`
		env.addFlow(t, "tool-bad-checksum", flowContent)
		jobID := env.enqueueJob(t, "tool-bad-checksum", nil)
		status := env.waitForJob(t, jobID, 10*time.Second)

		assert.Equal(t, queue.JobStatusFailed, status)
	})

	t.Run("missing PATH tool fails", func(t *testing.T) {
		env := setupE2EEnvironment(t)
		defer env.cleanup()

		flowContent := `runs-on: host
tools:
  - name: nonexistent-tool-xyz-123
steps:
  - cmd: echo "should not run"
    kind: shell
`
		env.addFlow(t, "tool-missing", flowContent)
		jobID := env.enqueueJob(t, "tool-missing", nil)
		status := env.waitForJob(t, jobID, 10*time.Second)

		assert.Equal(t, queue.JobStatusFailed, status)
	})
}
