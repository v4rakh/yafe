package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockAuthenticator is a simple authenticator for testing.
type mockAuthenticator struct {
	users map[string]*User
}

func newMockAuthenticator(users map[string]*User) *mockAuthenticator {
	return &mockAuthenticator{users: users}
}

func (m *mockAuthenticator) Authenticate(key string) *User {
	return m.users[key]
}

func TestMiddleware(t *testing.T) {
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := GetUserFromContext(r.Context())
		if user != nil {
			w.Write([]byte("user:" + user.Name))
		} else {
			w.Write([]byte("no-user"))
		}
	})

	t.Run("passes through when auth not required and no authenticator", func(t *testing.T) {
		handler := Middleware(nil, false)(testHandler)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "no-user", rec.Body.String())
	})

	t.Run("rejects when auth required but no authenticator", func(t *testing.T) {
		handler := Middleware(nil, true)(testHandler)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assertJSONError(t, rec, "authentication required")
	})

	t.Run("passes through when auth not required and no key provided", func(t *testing.T) {
		auth := newMockAuthenticator(map[string]*User{
			"valid-key": {Name: "testuser"},
		})
		handler := Middleware(auth, false)(testHandler)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "no-user", rec.Body.String())
	})

	t.Run("rejects when auth required and no key provided", func(t *testing.T) {
		auth := newMockAuthenticator(map[string]*User{
			"valid-key": {Name: "testuser"},
		})
		handler := Middleware(auth, true)(testHandler)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assertJSONError(t, rec, "authentication required")
	})

	t.Run("rejects invalid key", func(t *testing.T) {
		auth := newMockAuthenticator(map[string]*User{
			"valid-key": {Name: "testuser"},
		})
		handler := Middleware(auth, true)(testHandler)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set(HeaderAPIKey, "invalid-key")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assertJSONError(t, rec, "invalid API key")
	})

	t.Run("attaches user to context for valid key", func(t *testing.T) {
		auth := newMockAuthenticator(map[string]*User{
			"valid-key": {Name: "testuser", Roles: []Role{RoleJobsread}},
		})
		handler := Middleware(auth, true)(testHandler)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set(HeaderAPIKey, "valid-key")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "user:testuser", rec.Body.String())
	})

	t.Run("attaches user to context when auth optional", func(t *testing.T) {
		auth := newMockAuthenticator(map[string]*User{
			"valid-key": {Name: "testuser"},
		})
		handler := Middleware(auth, false)(testHandler)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set(HeaderAPIKey, "valid-key")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "user:testuser", rec.Body.String())
	})
}

func TestRequireRole(t *testing.T) {
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	t.Run("passes through when user not in context", func(t *testing.T) {
		handler := RequireRole(RoleJobsread)(testHandler)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("allows user with required role", func(t *testing.T) {
		handler := RequireRole(RoleJobsread)(testHandler)

		user := &User{Name: "test", Roles: []Role{RoleJobsread}}
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req = req.WithContext(WithUser(req.Context(), user))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "success", rec.Body.String())
	})

	t.Run("allows user with multiple roles including required", func(t *testing.T) {
		handler := RequireRole(RoleFlowsread)(testHandler)

		user := &User{Name: "test", Roles: []Role{RoleJobsread, RoleFlowsread, RoleFlowswrite}}
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req = req.WithContext(WithUser(req.Context(), user))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("rejects user without required role", func(t *testing.T) {
		handler := RequireRole(RoleJobswrite)(testHandler)

		user := &User{Name: "test", Roles: []Role{RoleJobsread}}
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req = req.WithContext(WithUser(req.Context(), user))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusForbidden, rec.Code)
		assertJSONError(t, rec, "insufficient permissions")
	})

	t.Run("rejects user with no roles", func(t *testing.T) {
		handler := RequireRole(RoleJobsread)(testHandler)

		user := &User{Name: "test", Roles: nil}
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req = req.WithContext(WithUser(req.Context(), user))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusForbidden, rec.Code)
	})
}

func TestLogAccess(t *testing.T) {
	t.Run("calls next handler", func(t *testing.T) {
		called := false
		testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		})

		handler := LogAccess(testHandler)
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.True(t, called)
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestMiddlewareChain(t *testing.T) {
	// Test the full chain: LogAccess -> Middleware -> RequireRole -> Handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	t.Run("full chain allows authenticated user with correct role", func(t *testing.T) {
		auth := newMockAuthenticator(map[string]*User{
			"valid-key": {Name: "admin", Roles: []Role{RoleJobsread, RoleJobswrite}},
		})

		handler := LogAccess(
			Middleware(auth, true)(
				RequireRole(RoleJobswrite)(testHandler),
			),
		)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", nil)
		req.Header.Set(HeaderAPIKey, "valid-key")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "success", rec.Body.String())
	})

	t.Run("full chain rejects unauthenticated user", func(t *testing.T) {
		auth := newMockAuthenticator(map[string]*User{
			"valid-key": {Name: "admin", Roles: []Role{RoleJobsread}},
		})

		handler := LogAccess(
			Middleware(auth, true)(
				RequireRole(RoleJobsread)(testHandler),
			),
		)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("full chain rejects user with wrong role", func(t *testing.T) {
		auth := newMockAuthenticator(map[string]*User{
			"valid-key": {Name: "reader", Roles: []Role{RoleJobsread}},
		})

		handler := LogAccess(
			Middleware(auth, true)(
				RequireRole(RoleJobswrite)(testHandler),
			),
		)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", nil)
		req.Header.Set(HeaderAPIKey, "valid-key")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusForbidden, rec.Code)
	})
}

func assertJSONError(t *testing.T, rec *httptest.ResponseRecorder, expectedMsg string) {
	t.Helper()
	var resp errorResponse
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, expectedMsg, resp.Error)
}
