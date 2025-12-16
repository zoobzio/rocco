package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zoobzio/rocco"
)

type counterOutput struct {
	Count int `json:"count"`
}

type idOutput struct {
	ID string `json:"id"`
}

// TestConcurrency_ParallelRequests tests handling many concurrent requests.
func TestConcurrency_ParallelRequests(t *testing.T) {
	engine := rocco.NewEngine("localhost", 0, nil)

	var counter int64
	handler := rocco.NewHandler[rocco.NoBody, counterOutput](
		"counter",
		"GET",
		"/count",
		func(_ *rocco.Request[rocco.NoBody]) (counterOutput, error) {
			atomic.AddInt64(&counter, 1)
			return counterOutput{Count: int(atomic.LoadInt64(&counter))}, nil
		},
	)
	engine.WithHandlers(handler)

	const numRequests = 100
	var wg sync.WaitGroup
	results := make(chan int, numRequests)
	errors := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/count", nil)
			w := httptest.NewRecorder()
			engine.Router().ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				errors <- fmt.Errorf("expected 200, got %d", w.Code)
				return
			}

			var resp counterOutput
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				errors <- err
				return
			}
			results <- resp.Count
		}()
	}

	wg.Wait()
	close(results)
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("request error: %v", err)
	}

	// Verify counter reached expected value
	finalCount := atomic.LoadInt64(&counter)
	if finalCount != numRequests {
		t.Errorf("expected counter %d, got %d", numRequests, finalCount)
	}
}

// TestConcurrency_DifferentHandlers tests concurrent requests to different handlers.
func TestConcurrency_DifferentHandlers(t *testing.T) {
	engine := rocco.NewEngine("localhost", 0, nil)

	// Register multiple handlers
	for i := 0; i < 10; i++ {
		idx := i
		handler := rocco.NewHandler[rocco.NoBody, idOutput](
			fmt.Sprintf("handler-%d", i),
			"GET",
			fmt.Sprintf("/endpoint%d", i),
			func(_ *rocco.Request[rocco.NoBody]) (idOutput, error) {
				return idOutput{ID: fmt.Sprintf("handler-%d", idx)}, nil
			},
		)
		engine.WithHandlers(handler)
	}

	const requestsPerHandler = 20
	var wg sync.WaitGroup
	errors := make(chan error, 10*requestsPerHandler)

	for i := 0; i < 10; i++ {
		endpoint := fmt.Sprintf("/endpoint%d", i)
		expectedID := fmt.Sprintf("handler-%d", i)

		for j := 0; j < requestsPerHandler; j++ {
			wg.Add(1)
			go func(ep, expID string) {
				defer wg.Done()

				req := httptest.NewRequest("GET", ep, nil)
				w := httptest.NewRecorder()
				engine.Router().ServeHTTP(w, req)

				if w.Code != http.StatusOK {
					errors <- fmt.Errorf("endpoint %s: expected 200, got %d", ep, w.Code)
					return
				}

				var resp idOutput
				if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
					errors <- fmt.Errorf("endpoint %s: decode error: %v", ep, err)
					return
				}

				if resp.ID != expID {
					errors <- fmt.Errorf("endpoint %s: expected ID %q, got %q", ep, expID, resp.ID)
				}
			}(endpoint, expectedID)
		}
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Error(err)
	}
}

// TestConcurrency_WithMiddleware tests concurrent requests with middleware.
func TestConcurrency_WithMiddleware(t *testing.T) {
	engine := rocco.NewEngine("localhost", 0, nil)

	var middlewareCount int64
	middleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&middlewareCount, 1)
			next.ServeHTTP(w, r)
		})
	}
	engine.WithMiddleware(middleware)

	handler := rocco.NewHandler[rocco.NoBody, idOutput](
		"test",
		"GET",
		"/test",
		func(_ *rocco.Request[rocco.NoBody]) (idOutput, error) {
			return idOutput{ID: "ok"}, nil
		},
	)
	engine.WithHandlers(handler)

	const numRequests = 50
	var wg sync.WaitGroup

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()
			engine.Router().ServeHTTP(w, req)
		}()
	}

	wg.Wait()

	if middlewareCount != numRequests {
		t.Errorf("expected middleware count %d, got %d", numRequests, middlewareCount)
	}
}

// TestConcurrency_WithAuthentication tests concurrent authenticated requests.
func TestConcurrency_WithAuthentication(t *testing.T) {
	var authCount int64
	engine := rocco.NewEngine("localhost", 0, func(_ context.Context, r *http.Request) (rocco.Identity, error) {
		atomic.AddInt64(&authCount, 1)
		token := r.Header.Get("Authorization")
		if token == "" {
			return nil, fmt.Errorf("missing token")
		}
		return &testIdentity{id: token}, nil
	})

	handler := rocco.NewHandler[rocco.NoBody, idOutput](
		"protected",
		"GET",
		"/protected",
		func(req *rocco.Request[rocco.NoBody]) (idOutput, error) {
			return idOutput{ID: req.Identity.ID()}, nil
		},
	).WithAuthentication()
	engine.WithHandlers(handler)

	const numRequests = 50
	var wg sync.WaitGroup
	errors := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			token := fmt.Sprintf("token-%d", idx)
			req := httptest.NewRequest("GET", "/protected", nil)
			req.Header.Set("Authorization", token)
			w := httptest.NewRecorder()
			engine.Router().ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				errors <- fmt.Errorf("request %d: expected 200, got %d", idx, w.Code)
				return
			}

			var resp idOutput
			json.Unmarshal(w.Body.Bytes(), &resp)
			if resp.ID != token {
				errors <- fmt.Errorf("request %d: expected ID %q, got %q", idx, token, resp.ID)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Error(err)
	}

	if authCount != numRequests {
		t.Errorf("expected auth count %d, got %d", numRequests, authCount)
	}
}

// TestConcurrency_ErrorHandling tests concurrent requests that return errors.
func TestConcurrency_ErrorHandling(t *testing.T) {
	engine := rocco.NewEngine("localhost", 0, nil)

	var counter int64
	handler := rocco.NewHandler[rocco.NoBody, idOutput](
		"alternating",
		"GET",
		"/alternating",
		func(_ *rocco.Request[rocco.NoBody]) (idOutput, error) {
			count := atomic.AddInt64(&counter, 1)
			if count%2 == 0 {
				return idOutput{}, rocco.ErrNotFound.WithMessage("not found")
			}
			return idOutput{ID: "found"}, nil
		},
	).WithErrors(rocco.ErrNotFound)
	engine.WithHandlers(handler)

	const numRequests = 100
	var wg sync.WaitGroup
	var successCount, errorCount int64

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/alternating", nil)
			w := httptest.NewRecorder()
			engine.Router().ServeHTTP(w, req)

			switch w.Code {
			case http.StatusOK:
				atomic.AddInt64(&successCount, 1)
			case http.StatusNotFound:
				atomic.AddInt64(&errorCount, 1)
			default:
				t.Errorf("unexpected status: %d", w.Code)
			}
		}()
	}

	wg.Wait()

	total := successCount + errorCount
	if total != numRequests {
		t.Errorf("expected total %d, got %d (success: %d, error: %d)", numRequests, total, successCount, errorCount)
	}
}

// TestConcurrency_BodyParsing tests concurrent requests with body parsing.
func TestConcurrency_BodyParsing(t *testing.T) {
	type echoInput struct {
		Message string `json:"message"`
	}
	type echoOutput struct {
		Echo string `json:"echo"`
	}

	engine := rocco.NewEngine("localhost", 0, nil)
	handler := rocco.NewHandler[echoInput, echoOutput](
		"echo",
		"POST",
		"/echo",
		func(req *rocco.Request[echoInput]) (echoOutput, error) {
			return echoOutput{Echo: req.Body.Message}, nil
		},
	)
	engine.WithHandlers(handler)

	const numRequests = 50
	var wg sync.WaitGroup
	errors := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			msg := fmt.Sprintf("message-%d", idx)
			body, _ := json.Marshal(echoInput{Message: msg})

			req := httptest.NewRequest("POST", "/echo", bytes.NewReader(body))
			w := httptest.NewRecorder()
			engine.Router().ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				errors <- fmt.Errorf("request %d: expected 200, got %d", idx, w.Code)
				return
			}

			var resp echoOutput
			json.Unmarshal(w.Body.Bytes(), &resp)
			if resp.Echo != msg {
				errors <- fmt.Errorf("request %d: expected echo %q, got %q", idx, msg, resp.Echo)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Error(err)
	}
}

// TestConcurrency_SlowHandlers tests concurrent slow handlers.
func TestConcurrency_SlowHandlers(t *testing.T) {
	engine := rocco.NewEngine("localhost", 0, nil)

	handler := rocco.NewHandler[rocco.NoBody, idOutput](
		"slow",
		"GET",
		"/slow",
		func(_ *rocco.Request[rocco.NoBody]) (idOutput, error) {
			time.Sleep(10 * time.Millisecond)
			return idOutput{ID: "done"}, nil
		},
	)
	engine.WithHandlers(handler)

	const numRequests = 20
	var wg sync.WaitGroup
	start := time.Now()

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/slow", nil)
			w := httptest.NewRecorder()
			engine.Router().ServeHTTP(w, req)
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)

	// All requests should complete much faster than sequential (20 * 10ms = 200ms)
	// Allow for some overhead but should be < 100ms with parallelism
	if elapsed > 100*time.Millisecond {
		t.Logf("requests completed in %v (may indicate lack of parallelism)", elapsed)
	}
}

// testIdentity implements rocco.Identity for testing.
type testIdentity struct {
	id       string
	tenantID string
	scopes   []string
	roles    []string
}

func (t *testIdentity) ID() string            { return t.id }
func (t *testIdentity) TenantID() string      { return t.tenantID }
func (t *testIdentity) Scopes() []string      { return t.scopes }
func (t *testIdentity) Roles() []string       { return t.roles }
func (t *testIdentity) Stats() map[string]int { return nil }

func (t *testIdentity) HasScope(scope string) bool {
	for _, s := range t.scopes {
		if s == scope {
			return true
		}
	}
	return false
}

func (t *testIdentity) HasRole(role string) bool {
	for _, r := range t.roles {
		if r == role {
			return true
		}
	}
	return false
}
