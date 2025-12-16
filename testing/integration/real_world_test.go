package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/zoobzio/rocco"
)

// Domain types for real-world scenarios.

type User struct {
	ID    string `json:"id"`
	Name  string `json:"name" validate:"required,min=1,max=100"`
	Email string `json:"email" validate:"required,email"`
}

type CreateUserInput struct {
	Name  string `json:"name" validate:"required,min=1,max=100"`
	Email string `json:"email" validate:"required,email"`
}

type UpdateUserInput struct {
	Name  string `json:"name" validate:"omitempty,min=1,max=100"`
	Email string `json:"email" validate:"omitempty,email"`
}

type UserListOutput struct {
	Users []User `json:"users"`
	Total int    `json:"total"`
}

type DeleteOutput struct {
	Deleted bool `json:"deleted"`
}

// In-memory store for testing.
type userStore struct {
	mu     sync.RWMutex
	users  map[string]*User
	nextID int
}

func newUserStore() *userStore {
	return &userStore{
		users:  make(map[string]*User),
		nextID: 1,
	}
}

func (s *userStore) Create(name, email string) *User {
	s.mu.Lock()
	defer s.mu.Unlock()
	user := &User{
		ID:    fmt.Sprintf("user-%d", s.nextID),
		Name:  name,
		Email: email,
	}
	s.nextID++
	s.users[user.ID] = user
	return user
}

func (s *userStore) Get(id string) *User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.users[id]
}

func (s *userStore) Update(id, name, email string) *User {
	s.mu.Lock()
	defer s.mu.Unlock()
	user := s.users[id]
	if user == nil {
		return nil
	}
	if name != "" {
		user.Name = name
	}
	if email != "" {
		user.Email = email
	}
	return user
}

func (s *userStore) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.users[id]; exists {
		delete(s.users, id)
		return true
	}
	return false
}

func (s *userStore) List() []*User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	users := make([]*User, 0, len(s.users))
	for _, u := range s.users {
		users = append(users, u)
	}
	return users
}

// TestRealWorld_CRUDOperations tests a complete CRUD workflow.
func TestRealWorld_CRUDOperations(t *testing.T) {
	store := newUserStore()
	engine := rocco.NewEngine("localhost", 0, nil)

	// Create handler
	createHandler := rocco.NewHandler[CreateUserInput, User](
		"create-user",
		"POST",
		"/users",
		func(req *rocco.Request[CreateUserInput]) (User, error) {
			user := store.Create(req.Body.Name, req.Body.Email)
			return *user, nil
		},
	).WithSuccessStatus(http.StatusCreated)

	// List handler
	listHandler := rocco.NewHandler[rocco.NoBody, UserListOutput](
		"list-users",
		"GET",
		"/users",
		func(_ *rocco.Request[rocco.NoBody]) (UserListOutput, error) {
			users := store.List()
			result := UserListOutput{Total: len(users)}
			for _, u := range users {
				result.Users = append(result.Users, *u)
			}
			return result, nil
		},
	)

	// Get handler
	getHandler := rocco.NewHandler[rocco.NoBody, User](
		"get-user",
		"GET",
		"/users/{id}",
		func(req *rocco.Request[rocco.NoBody]) (User, error) {
			user := store.Get(req.Params.Path["id"])
			if user == nil {
				return User{}, rocco.ErrNotFound.WithMessage("user not found")
			}
			return *user, nil
		},
	).WithPathParams("id").
		WithErrors(rocco.ErrNotFound)

	// Update handler
	updateHandler := rocco.NewHandler[UpdateUserInput, User](
		"update-user",
		"PUT",
		"/users/{id}",
		func(req *rocco.Request[UpdateUserInput]) (User, error) {
			user := store.Update(req.Params.Path["id"], req.Body.Name, req.Body.Email)
			if user == nil {
				return User{}, rocco.ErrNotFound.WithMessage("user not found")
			}
			return *user, nil
		},
	).WithPathParams("id").
		WithErrors(rocco.ErrNotFound)

	// Delete handler
	deleteHandler := rocco.NewHandler[rocco.NoBody, DeleteOutput](
		"delete-user",
		"DELETE",
		"/users/{id}",
		func(req *rocco.Request[rocco.NoBody]) (DeleteOutput, error) {
			deleted := store.Delete(req.Params.Path["id"])
			if !deleted {
				return DeleteOutput{}, rocco.ErrNotFound.WithMessage("user not found")
			}
			return DeleteOutput{Deleted: true}, nil
		},
	).WithPathParams("id").
		WithErrors(rocco.ErrNotFound)

	engine.WithHandlers(createHandler, listHandler, getHandler, updateHandler, deleteHandler)

	// Test Create
	t.Run("Create", func(t *testing.T) {
		body, _ := json.Marshal(CreateUserInput{Name: "John Doe", Email: "john@example.com"})
		req := httptest.NewRequest("POST", "/users", bytes.NewReader(body))
		w := httptest.NewRecorder()
		engine.Router().ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
		}

		var user User
		json.Unmarshal(w.Body.Bytes(), &user)
		if user.ID == "" {
			t.Error("expected user ID")
		}
		if user.Name != "John Doe" {
			t.Errorf("expected name 'John Doe', got %q", user.Name)
		}
	})

	// Test List
	t.Run("List", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/users", nil)
		w := httptest.NewRecorder()
		engine.Router().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}

		var list UserListOutput
		json.Unmarshal(w.Body.Bytes(), &list)
		if list.Total != 1 {
			t.Errorf("expected 1 user, got %d", list.Total)
		}
	})

	// Test Get
	t.Run("Get", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/users/user-1", nil)
		w := httptest.NewRecorder()
		engine.Router().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var user User
		json.Unmarshal(w.Body.Bytes(), &user)
		if user.ID != "user-1" {
			t.Errorf("expected ID 'user-1', got %q", user.ID)
		}
	})

	// Test Get Not Found
	t.Run("GetNotFound", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/users/nonexistent", nil)
		w := httptest.NewRecorder()
		engine.Router().ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", w.Code)
		}
	})

	// Test Update
	t.Run("Update", func(t *testing.T) {
		body, _ := json.Marshal(UpdateUserInput{Name: "Jane Doe"})
		req := httptest.NewRequest("PUT", "/users/user-1", bytes.NewReader(body))
		w := httptest.NewRecorder()
		engine.Router().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var user User
		json.Unmarshal(w.Body.Bytes(), &user)
		if user.Name != "Jane Doe" {
			t.Errorf("expected name 'Jane Doe', got %q", user.Name)
		}
	})

	// Test Delete
	t.Run("Delete", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/users/user-1", nil)
		w := httptest.NewRecorder()
		engine.Router().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}

		var result DeleteOutput
		json.Unmarshal(w.Body.Bytes(), &result)
		if !result.Deleted {
			t.Error("expected deleted=true")
		}
	})

	// Test Delete Not Found (already deleted)
	t.Run("DeleteNotFound", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/users/user-1", nil)
		w := httptest.NewRecorder()
		engine.Router().ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", w.Code)
		}
	})
}

// TestRealWorld_AuthenticationFlow tests authentication scenarios.
func TestRealWorld_AuthenticationFlow(t *testing.T) {
	validTokens := map[string]*testIdentity{
		"token-admin": {id: "admin-1", roles: []string{"admin", "user"}},
		"token-user":  {id: "user-1", roles: []string{"user"}},
	}

	engine := rocco.NewEngine("localhost", 0, func(_ context.Context, r *http.Request) (rocco.Identity, error) {
		token := r.Header.Get("Authorization")
		if identity, ok := validTokens[token]; ok {
			return identity, nil
		}
		return nil, fmt.Errorf("invalid token")
	})

	// Public endpoint
	publicHandler := rocco.NewHandler[rocco.NoBody, idOutput](
		"public",
		"GET",
		"/public",
		func(_ *rocco.Request[rocco.NoBody]) (idOutput, error) {
			return idOutput{ID: "public-data"}, nil
		},
	)

	// Protected endpoint (any authenticated user)
	protectedHandler := rocco.NewHandler[rocco.NoBody, idOutput](
		"protected",
		"GET",
		"/protected",
		func(req *rocco.Request[rocco.NoBody]) (idOutput, error) {
			return idOutput{ID: req.Identity.ID()}, nil
		},
	).WithAuthentication()

	// Admin-only endpoint
	adminHandler := rocco.NewHandler[rocco.NoBody, idOutput](
		"admin",
		"GET",
		"/admin",
		func(req *rocco.Request[rocco.NoBody]) (idOutput, error) {
			return idOutput{ID: req.Identity.ID()}, nil
		},
	).WithRoles("admin")

	engine.WithHandlers(publicHandler, protectedHandler, adminHandler)

	// Test public endpoint (no auth needed)
	t.Run("PublicNoAuth", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/public", nil)
		w := httptest.NewRecorder()
		engine.Router().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
	})

	// Test protected endpoint without auth
	t.Run("ProtectedNoAuth", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/protected", nil)
		w := httptest.NewRecorder()
		engine.Router().ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", w.Code)
		}
	})

	// Test protected endpoint with valid auth
	t.Run("ProtectedWithAuth", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/protected", nil)
		req.Header.Set("Authorization", "token-user")
		w := httptest.NewRecorder()
		engine.Router().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp idOutput
		json.Unmarshal(w.Body.Bytes(), &resp)
		if resp.ID != "user-1" {
			t.Errorf("expected ID 'user-1', got %q", resp.ID)
		}
	})

	// Test admin endpoint with user token
	t.Run("AdminWithUserToken", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin", nil)
		req.Header.Set("Authorization", "token-user")
		w := httptest.NewRecorder()
		engine.Router().ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d: %s", w.Code, w.Body.String())
		}
	})

	// Test admin endpoint with admin token
	t.Run("AdminWithAdminToken", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin", nil)
		req.Header.Set("Authorization", "token-admin")
		w := httptest.NewRecorder()
		engine.Router().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})
}

// TestRealWorld_ValidationErrors tests validation error handling.
func TestRealWorld_ValidationErrors(t *testing.T) {
	engine := rocco.NewEngine("localhost", 0, nil)

	handler := rocco.NewHandler[CreateUserInput, User](
		"create-user",
		"POST",
		"/users",
		func(_ *rocco.Request[CreateUserInput]) (User, error) {
			return User{}, nil
		},
	)
	engine.WithHandlers(handler)

	tests := []struct {
		name       string
		input      map[string]any
		wantStatus int
	}{
		{
			name:       "MissingName",
			input:      map[string]any{"email": "test@example.com"},
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:       "MissingEmail",
			input:      map[string]any{"name": "John"},
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:       "InvalidEmail",
			input:      map[string]any{"name": "John", "email": "not-an-email"},
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:       "NameTooLong",
			input:      map[string]any{"name": string(make([]byte, 150)), "email": "test@example.com"},
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:       "ValidInput",
			input:      map[string]any{"name": "John", "email": "john@example.com"},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.input)
			req := httptest.NewRequest("POST", "/users", bytes.NewReader(body))
			w := httptest.NewRecorder()
			engine.Router().ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("expected %d, got %d: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

// TestRealWorld_ErrorResponses tests structured error responses.
func TestRealWorld_ErrorResponses(t *testing.T) {
	engine := rocco.NewEngine("localhost", 0, nil)

	handler := rocco.NewHandler[rocco.NoBody, idOutput](
		"error-types",
		"GET",
		"/error/{type}",
		func(req *rocco.Request[rocco.NoBody]) (idOutput, error) {
			switch req.Params.Path["type"] {
			case "not-found":
				return idOutput{}, rocco.ErrNotFound.WithMessage("resource not found")
			case "bad-request":
				return idOutput{}, rocco.ErrBadRequest.WithMessage("invalid request")
			case "conflict":
				return idOutput{}, rocco.ErrConflict.WithMessage("resource already exists")
			default:
				return idOutput{ID: "ok"}, nil
			}
		},
	).WithPathParams("type").
		WithErrors(rocco.ErrNotFound, rocco.ErrBadRequest, rocco.ErrConflict)
	engine.WithHandlers(handler)

	tests := []struct {
		errorType  string
		wantStatus int
		wantCode   string
	}{
		{"not-found", http.StatusNotFound, "NOT_FOUND"},
		{"bad-request", http.StatusBadRequest, "BAD_REQUEST"},
		{"conflict", http.StatusConflict, "CONFLICT"},
		{"none", http.StatusOK, ""},
	}

	for _, tt := range tests {
		t.Run(tt.errorType, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/error/"+tt.errorType, nil)
			w := httptest.NewRecorder()
			engine.Router().ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, w.Code)
			}

			if tt.wantCode != "" {
				var resp struct {
					Code string `json:"code"`
				}
				json.Unmarshal(w.Body.Bytes(), &resp)
				if resp.Code != tt.wantCode {
					t.Errorf("expected code %q, got %q", tt.wantCode, resp.Code)
				}
			}
		})
	}
}

// TestRealWorld_MiddlewareChain tests a realistic middleware chain.
func TestRealWorld_MiddlewareChain(t *testing.T) {
	var order []string
	var mu sync.Mutex

	addToOrder := func(name string) {
		mu.Lock()
		order = append(order, name)
		mu.Unlock()
	}

	engine := rocco.NewEngine("localhost", 0, nil)

	// Logging middleware (engine level)
	engine.WithMiddleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			addToOrder("logging-start")
			next.ServeHTTP(w, r)
			addToOrder("logging-end")
		})
	})

	// Recovery middleware (engine level)
	engine.WithMiddleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			addToOrder("recovery-start")
			next.ServeHTTP(w, r)
			addToOrder("recovery-end")
		})
	})

	// Handler with its own middleware
	handler := rocco.NewHandler[rocco.NoBody, idOutput](
		"test",
		"GET",
		"/test",
		func(_ *rocco.Request[rocco.NoBody]) (idOutput, error) {
			addToOrder("handler")
			return idOutput{ID: "ok"}, nil
		},
	).WithMiddleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			addToOrder("handler-mw-start")
			next.ServeHTTP(w, r)
			addToOrder("handler-mw-end")
		})
	})
	engine.WithHandlers(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	engine.Router().ServeHTTP(w, req)

	expected := []string{
		"logging-start",
		"recovery-start",
		"handler-mw-start",
		"handler",
		"handler-mw-end",
		"recovery-end",
		"logging-end",
	}

	if len(order) != len(expected) {
		t.Errorf("expected %d middleware calls, got %d: %v", len(expected), len(order), order)
		return
	}

	for i, exp := range expected {
		if order[i] != exp {
			t.Errorf("position %d: expected %q, got %q", i, exp, order[i])
		}
	}
}

// TestRealWorld_QueryParameters tests query parameter handling.
func TestRealWorld_QueryParameters(t *testing.T) {
	type searchOutput struct {
		Query  string `json:"query"`
		Limit  string `json:"limit"`
		Offset string `json:"offset"`
	}

	engine := rocco.NewEngine("localhost", 0, nil)
	handler := rocco.NewHandler[rocco.NoBody, searchOutput](
		"search",
		"GET",
		"/search",
		func(req *rocco.Request[rocco.NoBody]) (searchOutput, error) {
			return searchOutput{
				Query:  req.Params.Query["q"],
				Limit:  req.Params.Query["limit"],
				Offset: req.Params.Query["offset"],
			}, nil
		},
	).WithQueryParams("q", "limit", "offset")
	engine.WithHandlers(handler)

	tests := []struct {
		name   string
		url    string
		expect searchOutput
	}{
		{
			name:   "AllParams",
			url:    "/search?q=test&limit=10&offset=20",
			expect: searchOutput{Query: "test", Limit: "10", Offset: "20"},
		},
		{
			name:   "PartialParams",
			url:    "/search?q=test",
			expect: searchOutput{Query: "test", Limit: "", Offset: ""},
		},
		{
			name:   "NoParams",
			url:    "/search",
			expect: searchOutput{Query: "", Limit: "", Offset: ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.url, nil)
			w := httptest.NewRecorder()
			engine.Router().ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("expected 200, got %d", w.Code)
			}

			var resp searchOutput
			json.Unmarshal(w.Body.Bytes(), &resp)

			if resp != tt.expect {
				t.Errorf("expected %+v, got %+v", tt.expect, resp)
			}
		})
	}
}
