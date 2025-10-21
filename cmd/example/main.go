package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zoobzio/rocco"
)

// Request/Response types
type HelloRequest struct {
	Name string `json:"name" validate:"min=1,max=100" description:"Name to greet" example:"World"`
}

type HelloResponse struct {
	Message string `json:"message" description:"Greeting message" example:"Hello, World!"`
	Time    string `json:"time" validate:"datetime" description:"Response timestamp" example:"2025-01-01T12:00:00Z"`
}

// Handler function
func handleHello(req *rocco.Request[HelloRequest]) (HelloResponse, error) {
	name := req.Body.Name
	if name == "" {
		name = "World"
	}

	// Can access embedded http.Request if needed (e.g., for headers)
	userAgent := req.Header.Get("User-Agent")
	log.Printf("Request from: %s", userAgent)

	return HelloResponse{
		Message: fmt.Sprintf("Hello, %s!", name),
		Time:    time.Now().Format(time.RFC3339),
	}, nil
}

// Path param example
type UserResponse struct {
	UserID  string `json:"user_id" validate:"uuid4" description:"Unique user identifier" example:"550e8400-e29b-41d4-a716-446655440000"`
	Message string `json:"message" description:"Status message"`
	Verbose bool   `json:"verbose,omitempty" description:"Detailed response flag"`
}

func handleUser(req *rocco.Request[rocco.NoBody]) (UserResponse, error) {
	userID := req.Params.Path["id"]

	// Demonstrate sentinel error usage
	if userID == "404" {
		// Handler will return: {"error":"Not Found"}
		return UserResponse{}, rocco.ErrNotFound
	}

	// Query params are optional - empty string if not provided
	verbose := req.Params.Query["verbose"] == "true"

	return UserResponse{
		UserID:  userID,
		Message: fmt.Sprintf("User %s retrieved", userID),
		Verbose: verbose,
	}, nil
}

func main() {
	// Create engine configuration
	config := rocco.DefaultConfig().
		WithPort(8081)

	// Create and configure engine
	engine := rocco.NewEngine(config)

	// Register handlers
	helloHandler := rocco.NewHandler[HelloRequest, HelloResponse](
		"hello",
		"POST",
		"/hello",
		handleHello,
	).WithSummary("Say hello").WithTags("greetings")

	userHandler := rocco.NewHandler[rocco.NoBody, UserResponse](
		"get-user",
		"GET",
		"/users/{id}",
		handleUser,
	).WithSummary("Get user by ID").
		WithTags("users").
		WithPathParams("id").
		WithQueryParams("verbose"). // Declares optional query param
		WithErrorCodes(404)         // Declares this handler may return 404

	engine.Register(helloHandler, userHandler)

	// Register OpenAPI spec endpoint
	info := rocco.Info{
		Title:       "Example API",
		Version:     "1.0.0",
		Description: "Example Rocco HTTP API",
	}
	engine.RegisterOpenAPIHandler(info)
	engine.RegisterDocsHandler("/docs", "/openapi")

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start server in background
	serverErrors := make(chan error, 1)
	go func() {
		fmt.Println("[Main] Press Ctrl+C to shutdown gracefully")
		serverErrors <- engine.Start() // This blocks until server stops
	}()

	// Wait for interrupt signal or server error
	select {
	case err := <-serverErrors:
		log.Fatalf("[Main] Server error: %v", err)
	case sig := <-sigChan:
		fmt.Printf("\n[Main] Received signal: %v\n", sig)

		// Create shutdown context with timeout
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Shutdown engine (handles both server and workers)
		if err := engine.Shutdown(shutdownCtx); err != nil {
			log.Printf("[Main] Shutdown error: %v", err)
		}

		fmt.Println("[Main] Shutdown complete")
	}
}
