package rocco

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestAuthz_WithScopes_SingleGroup tests single scope group (OR logic within group).
func TestAuthz_WithScopes_SingleGroup(t *testing.T) {
	setupSyncMode(t)

	tests := []struct {
		name           string
		requiredScopes []string
		userScopes     []string
		expectStatus   int
	}{
		{
			name:           "has first scope",
			requiredScopes: []string{"read", "write"},
			userScopes:     []string{"read"},
			expectStatus:   http.StatusOK,
		},
		{
			name:           "has second scope",
			requiredScopes: []string{"read", "write"},
			userScopes:     []string{"write"},
			expectStatus:   http.StatusOK,
		},
		{
			name:           "has both scopes",
			requiredScopes: []string{"read", "write"},
			userScopes:     []string{"read", "write"},
			expectStatus:   http.StatusOK,
		},
		{
			name:           "has neither scope",
			requiredScopes: []string{"read", "write"},
			userScopes:     []string{"admin"},
			expectStatus:   http.StatusForbidden,
		},
		{
			name:           "has no scopes",
			requiredScopes: []string{"read", "write"},
			userScopes:     []string{},
			expectStatus:   http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := NewEngine("localhost", 8080, func(_ context.Context, _ *http.Request) (Identity, error) {
				return &testIdentity{scopes: tt.userScopes}, nil
			})

			handler := NewHandler[NoBody, testOutput](
				"scope-test",
				"GET",
				"/test",
				func(_ *Request[NoBody]) (testOutput, error) {
					return testOutput{Message: "ok"}, nil
				},
			).WithScopes(tt.requiredScopes...)

			engine.WithHandlers(handler)

			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()
			engine.chiRouter.ServeHTTP(w, req)

			if w.Code != tt.expectStatus {
				t.Errorf("expected status %d, got %d", tt.expectStatus, w.Code)
			}
		})
	}
}

// TestAuthz_WithScopes_MultipleGroups tests multiple scope groups (AND logic across groups).
func TestAuthz_WithScopes_MultipleGroups(t *testing.T) {
	setupSyncMode(t)

	tests := []struct {
		name         string
		group1       []string
		group2       []string
		userScopes   []string
		expectStatus int
	}{
		{
			name:         "has scope from both groups",
			group1:       []string{"read", "write"},
			group2:       []string{"admin", "superadmin"},
			userScopes:   []string{"read", "admin"},
			expectStatus: http.StatusOK,
		},
		{
			name:         "has multiple scopes from both groups",
			group1:       []string{"read", "write"},
			group2:       []string{"admin", "superadmin"},
			userScopes:   []string{"read", "write", "admin", "superadmin"},
			expectStatus: http.StatusOK,
		},
		{
			name:         "missing scope from first group",
			group1:       []string{"read", "write"},
			group2:       []string{"admin", "superadmin"},
			userScopes:   []string{"admin"},
			expectStatus: http.StatusForbidden,
		},
		{
			name:         "missing scope from second group",
			group1:       []string{"read", "write"},
			group2:       []string{"admin", "superadmin"},
			userScopes:   []string{"read"},
			expectStatus: http.StatusForbidden,
		},
		{
			name:         "has no scopes",
			group1:       []string{"read", "write"},
			group2:       []string{"admin", "superadmin"},
			userScopes:   []string{},
			expectStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := NewEngine("localhost", 8080, func(_ context.Context, _ *http.Request) (Identity, error) {
				return &testIdentity{scopes: tt.userScopes}, nil
			})

			handler := NewHandler[NoBody, testOutput](
				"scope-test",
				"GET",
				"/test",
				func(_ *Request[NoBody]) (testOutput, error) {
					return testOutput{Message: "ok"}, nil
				},
			).WithScopes(tt.group1...).WithScopes(tt.group2...)

			engine.WithHandlers(handler)

			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()
			engine.chiRouter.ServeHTTP(w, req)

			if w.Code != tt.expectStatus {
				t.Errorf("expected status %d, got %d", tt.expectStatus, w.Code)
			}
		})
	}
}

// TestAuthz_WithRoles_SingleGroup tests single role group (OR logic within group).
func TestAuthz_WithRoles_SingleGroup(t *testing.T) {
	setupSyncMode(t)

	tests := []struct {
		name          string
		requiredRoles []string
		userRoles     []string
		expectStatus  int
	}{
		{
			name:          "has first role",
			requiredRoles: []string{"user", "admin"},
			userRoles:     []string{"user"},
			expectStatus:  http.StatusOK,
		},
		{
			name:          "has second role",
			requiredRoles: []string{"user", "admin"},
			userRoles:     []string{"admin"},
			expectStatus:  http.StatusOK,
		},
		{
			name:          "has both roles",
			requiredRoles: []string{"user", "admin"},
			userRoles:     []string{"user", "admin"},
			expectStatus:  http.StatusOK,
		},
		{
			name:          "has neither role",
			requiredRoles: []string{"user", "admin"},
			userRoles:     []string{"guest"},
			expectStatus:  http.StatusForbidden,
		},
		{
			name:          "has no roles",
			requiredRoles: []string{"user", "admin"},
			userRoles:     []string{},
			expectStatus:  http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := NewEngine("localhost", 8080, func(_ context.Context, _ *http.Request) (Identity, error) {
				return &testIdentity{roles: tt.userRoles}, nil
			})

			handler := NewHandler[NoBody, testOutput](
				"role-test",
				"GET",
				"/test",
				func(_ *Request[NoBody]) (testOutput, error) {
					return testOutput{Message: "ok"}, nil
				},
			).WithRoles(tt.requiredRoles...)

			engine.WithHandlers(handler)

			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()
			engine.chiRouter.ServeHTTP(w, req)

			if w.Code != tt.expectStatus {
				t.Errorf("expected status %d, got %d", tt.expectStatus, w.Code)
			}
		})
	}
}

// TestAuthz_WithRoles_MultipleGroups tests multiple role groups (AND logic across groups).
func TestAuthz_WithRoles_MultipleGroups(t *testing.T) {
	setupSyncMode(t)

	tests := []struct {
		name         string
		group1       []string
		group2       []string
		userRoles    []string
		expectStatus int
	}{
		{
			name:         "has role from both groups",
			group1:       []string{"user", "member"},
			group2:       []string{"verified", "trusted"},
			userRoles:    []string{"user", "verified"},
			expectStatus: http.StatusOK,
		},
		{
			name:         "has multiple roles from both groups",
			group1:       []string{"user", "member"},
			group2:       []string{"verified", "trusted"},
			userRoles:    []string{"user", "member", "verified", "trusted"},
			expectStatus: http.StatusOK,
		},
		{
			name:         "missing role from first group",
			group1:       []string{"user", "member"},
			group2:       []string{"verified", "trusted"},
			userRoles:    []string{"verified"},
			expectStatus: http.StatusForbidden,
		},
		{
			name:         "missing role from second group",
			group1:       []string{"user", "member"},
			group2:       []string{"verified", "trusted"},
			userRoles:    []string{"user"},
			expectStatus: http.StatusForbidden,
		},
		{
			name:         "has no roles",
			group1:       []string{"user", "member"},
			group2:       []string{"verified", "trusted"},
			userRoles:    []string{},
			expectStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := NewEngine("localhost", 8080, func(_ context.Context, _ *http.Request) (Identity, error) {
				return &testIdentity{roles: tt.userRoles}, nil
			})

			handler := NewHandler[NoBody, testOutput](
				"role-test",
				"GET",
				"/test",
				func(_ *Request[NoBody]) (testOutput, error) {
					return testOutput{Message: "ok"}, nil
				},
			).WithRoles(tt.group1...).WithRoles(tt.group2...)

			engine.WithHandlers(handler)

			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()
			engine.chiRouter.ServeHTTP(w, req)

			if w.Code != tt.expectStatus {
				t.Errorf("expected status %d, got %d", tt.expectStatus, w.Code)
			}
		})
	}
}

// TestAuthz_WithScopesAndRoles tests combining scopes and roles (AND logic).
func TestAuthz_WithScopesAndRoles(t *testing.T) {
	setupSyncMode(t)

	tests := []struct {
		name         string
		scopes       []string
		roles        []string
		userScopes   []string
		userRoles    []string
		expectStatus int
	}{
		{
			name:         "has both scope and role",
			scopes:       []string{"read"},
			roles:        []string{"user"},
			userScopes:   []string{"read"},
			userRoles:    []string{"user"},
			expectStatus: http.StatusOK,
		},
		{
			name:         "missing scope",
			scopes:       []string{"read"},
			roles:        []string{"user"},
			userScopes:   []string{},
			userRoles:    []string{"user"},
			expectStatus: http.StatusForbidden,
		},
		{
			name:         "missing role",
			scopes:       []string{"read"},
			roles:        []string{"user"},
			userScopes:   []string{"read"},
			userRoles:    []string{},
			expectStatus: http.StatusForbidden,
		},
		{
			name:         "missing both",
			scopes:       []string{"read"},
			roles:        []string{"user"},
			userScopes:   []string{},
			userRoles:    []string{},
			expectStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := NewEngine("localhost", 8080, func(_ context.Context, _ *http.Request) (Identity, error) {
				return &testIdentity{
					scopes: tt.userScopes,
					roles:  tt.userRoles,
				}, nil
			})

			handler := NewHandler[NoBody, testOutput](
				"authz-test",
				"GET",
				"/test",
				func(_ *Request[NoBody]) (testOutput, error) {
					return testOutput{Message: "ok"}, nil
				},
			).WithScopes(tt.scopes...).WithRoles(tt.roles...)

			engine.WithHandlers(handler)

			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()
			engine.chiRouter.ServeHTTP(w, req)

			if w.Code != tt.expectStatus {
				t.Errorf("expected status %d, got %d", tt.expectStatus, w.Code)
			}
		})
	}
}

// TestAuthz_RequiresAuthImplied tests that WithScopes/WithRoles implies RequiresAuth.
func TestAuthz_RequiresAuthImplied(t *testing.T) {
	setupSyncMode(t)

	t.Run("WithScopes implies RequiresAuth", func(t *testing.T) {
		handler := NewHandler[NoBody, testOutput](
			"scope-handler",
			"GET",
			"/test",
			func(_ *Request[NoBody]) (testOutput, error) {
				return testOutput{Message: "ok"}, nil
			},
		).WithScopes("read")

		if !handler.RequiresAuth() {
			t.Error("expected RequiresAuth() to be true after WithScopes()")
		}
	})

	t.Run("WithRoles implies RequiresAuth", func(t *testing.T) {
		handler := NewHandler[NoBody, testOutput](
			"role-handler",
			"GET",
			"/test",
			func(_ *Request[NoBody]) (testOutput, error) {
				return testOutput{Message: "ok"}, nil
			},
		).WithRoles("user")

		if !handler.RequiresAuth() {
			t.Error("expected RequiresAuth() to be true after WithRoles()")
		}
	})
}

// TestAuthz_NoIdentityInContext tests behavior when identity is missing from context.
func TestAuthz_NoIdentityInContext(t *testing.T) {
	setupSyncMode(t)

	// Create engine with extractIdentity that doesn't add identity to context
	engine := NewEngine("localhost", 8080, func(_ context.Context, _ *http.Request) (Identity, error) {
		return &testIdentity{scopes: []string{"read"}}, nil
	})

	handler := NewHandler[NoBody, testOutput](
		"authz-test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "ok"}, nil
		},
	).WithScopes("read")

	engine.WithHandlers(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	engine.chiRouter.ServeHTTP(w, req)

	// Should succeed because auth middleware adds identity to context
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

// testIdentity is a test implementation of Identity.
type testIdentity struct {
	id       string
	tenantID string
	scopes   []string
	roles    []string
	stats    map[string]int
}

func (i *testIdentity) ID() string       { return i.id }
func (i *testIdentity) TenantID() string { return i.tenantID }

func (i *testIdentity) HasScope(scope string) bool {
	for _, s := range i.scopes {
		if s == scope {
			return true
		}
	}
	return false
}

func (i *testIdentity) HasRole(role string) bool {
	for _, r := range i.roles {
		if r == role {
			return true
		}
	}
	return false
}

func (i *testIdentity) Stats() map[string]int {
	return i.stats
}

// TestUsageLimit_BelowThreshold tests usage limit allows request when below threshold.
func TestUsageLimit_BelowThreshold(t *testing.T) {
	setupSyncMode(t)

	engine := NewEngine("localhost", 8080, func(_ context.Context, _ *http.Request) (Identity, error) {
		return &testIdentity{
			stats: map[string]int{
				"requests_today": 50,
			},
		}, nil
	})

	handler := NewHandler[NoBody, testOutput](
		"usage-test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "ok"}, nil
		},
	).WithUsageLimit("requests_today", func(Identity) int { return 100 })

	engine.WithHandlers(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	engine.chiRouter.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

// TestUsageLimit_AtThreshold tests usage limit denies request when at threshold.
func TestUsageLimit_AtThreshold(t *testing.T) {
	setupSyncMode(t)

	engine := NewEngine("localhost", 8080, func(_ context.Context, _ *http.Request) (Identity, error) {
		return &testIdentity{
			stats: map[string]int{
				"requests_today": 100,
			},
		}, nil
	})

	handler := NewHandler[NoBody, testOutput](
		"usage-test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "ok"}, nil
		},
	).WithUsageLimit("requests_today", func(Identity) int { return 100 })

	engine.WithHandlers(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	engine.chiRouter.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected status 429, got %d", w.Code)
	}
}

// TestUsageLimit_AboveThreshold tests usage limit denies request when above threshold.
func TestUsageLimit_AboveThreshold(t *testing.T) {
	setupSyncMode(t)

	engine := NewEngine("localhost", 8080, func(_ context.Context, _ *http.Request) (Identity, error) {
		return &testIdentity{
			stats: map[string]int{
				"requests_today": 150,
			},
		}, nil
	})

	handler := NewHandler[NoBody, testOutput](
		"usage-test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "ok"}, nil
		},
	).WithUsageLimit("requests_today", func(Identity) int { return 100 })

	engine.WithHandlers(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	engine.chiRouter.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected status 429, got %d", w.Code)
	}
}

// TestUsageLimit_MultipleLimits tests multiple usage limits (AND logic).
func TestUsageLimit_MultipleLimits(t *testing.T) {
	setupSyncMode(t)

	tests := []struct {
		name         string
		stats        map[string]int
		expectStatus int
	}{
		{
			name: "both below threshold",
			stats: map[string]int{
				"requests_today": 50,
				"api_calls":      25,
			},
			expectStatus: http.StatusOK,
		},
		{
			name: "first at threshold",
			stats: map[string]int{
				"requests_today": 100,
				"api_calls":      25,
			},
			expectStatus: http.StatusTooManyRequests,
		},
		{
			name: "second at threshold",
			stats: map[string]int{
				"requests_today": 50,
				"api_calls":      50,
			},
			expectStatus: http.StatusTooManyRequests,
		},
		{
			name: "both at threshold",
			stats: map[string]int{
				"requests_today": 100,
				"api_calls":      50,
			},
			expectStatus: http.StatusTooManyRequests,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := NewEngine("localhost", 8080, func(_ context.Context, _ *http.Request) (Identity, error) {
				return &testIdentity{stats: tt.stats}, nil
			})

			handler := NewHandler[NoBody, testOutput](
				"usage-test",
				"GET",
				"/test",
				func(_ *Request[NoBody]) (testOutput, error) {
					return testOutput{Message: "ok"}, nil
				},
			).WithUsageLimit("requests_today", func(Identity) int { return 100 }).WithUsageLimit("api_calls", func(Identity) int { return 50 })

			engine.WithHandlers(handler)

			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()
			engine.chiRouter.ServeHTTP(w, req)

			if w.Code != tt.expectStatus {
				t.Errorf("expected status %d, got %d", tt.expectStatus, w.Code)
			}
		})
	}
}

// TestUsageLimit_MissingStatKey tests usage limit when stat key is missing (treated as 0).
func TestUsageLimit_MissingStatKey(t *testing.T) {
	setupSyncMode(t)

	engine := NewEngine("localhost", 8080, func(_ context.Context, _ *http.Request) (Identity, error) {
		return &testIdentity{
			stats: map[string]int{
				"other_metric": 100,
			},
		}, nil
	})

	handler := NewHandler[NoBody, testOutput](
		"usage-test",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "ok"}, nil
		},
	).WithUsageLimit("requests_today", func(Identity) int { return 100 })

	engine.WithHandlers(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	engine.chiRouter.ServeHTTP(w, req)

	// Missing key treated as 0, so request should succeed
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

// TestUsageLimit_RequiresAuth tests that WithUsageLimit implies RequiresAuth.
func TestUsageLimit_RequiresAuth(t *testing.T) {
	setupSyncMode(t)

	handler := NewHandler[NoBody, testOutput](
		"usage-handler",
		"GET",
		"/test",
		func(_ *Request[NoBody]) (testOutput, error) {
			return testOutput{Message: "ok"}, nil
		},
	).WithUsageLimit("requests_today", func(Identity) int { return 100 })

	if !handler.RequiresAuth() {
		t.Error("expected RequiresAuth() to be true after WithUsageLimit()")
	}
}

// TestUsageLimit_DynamicThreshold tests usage limits with dynamic thresholds per identity.
func TestUsageLimit_DynamicThreshold(t *testing.T) {
	setupSyncMode(t)

	tests := []struct {
		name         string
		tenantID     string
		usage        int
		expectStatus int
	}{
		{
			name:         "premium tenant below limit",
			tenantID:     "premium",
			usage:        5000,
			expectStatus: http.StatusOK,
		},
		{
			name:         "premium tenant at limit",
			tenantID:     "premium",
			usage:        10000,
			expectStatus: http.StatusTooManyRequests,
		},
		{
			name:         "free tenant below limit",
			tenantID:     "free",
			usage:        500,
			expectStatus: http.StatusOK,
		},
		{
			name:         "free tenant at limit",
			tenantID:     "free",
			usage:        1000,
			expectStatus: http.StatusTooManyRequests,
		},
		{
			name:         "free tenant with premium usage blocked",
			tenantID:     "free",
			usage:        5000,
			expectStatus: http.StatusTooManyRequests,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := NewEngine("localhost", 8080, func(_ context.Context, _ *http.Request) (Identity, error) {
				return &testIdentity{
					tenantID: tt.tenantID,
					stats: map[string]int{
						"requests_today": tt.usage,
					},
				}, nil
			})

			handler := NewHandler[NoBody, testOutput](
				"usage-test",
				"GET",
				"/test",
				func(_ *Request[NoBody]) (testOutput, error) {
					return testOutput{Message: "ok"}, nil
				},
			).WithUsageLimit("requests_today", func(identity Identity) int {
				// Premium tenants get higher limits
				if identity.TenantID() == "premium" {
					return 10000
				}
				// Free tier
				return 1000
			})

			engine.WithHandlers(handler)

			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()
			engine.chiRouter.ServeHTTP(w, req)

			if w.Code != tt.expectStatus {
				t.Errorf("expected status %d, got %d", tt.expectStatus, w.Code)
			}
		})
	}
}
