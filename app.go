package main

import (
	"HRPMLW/internal/config"
	"HRPMLW/internal/database"
	"HRPMLW/internal/models"
	"HRPMLW/internal/routes"
	"HRPMLW/internal/services"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// App struct
type App struct {
	ctx        context.Context
	httpClient *services.HTTPService
	router     *routes.Router
	config     *config.Config
}

// NewApp creates a new App application struct
func NewApp() *App {
	cfg := config.Load()
	return &App{
		config:     cfg,
		httpClient: services.NewHTTPService("http://localhost:" + cfg.ServerPort),
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Initialize database
	if err := database.Initialize(database.DefaultConfig()); err != nil {
		fmt.Printf("Failed to initialize database: %v\n", err)
	}

	// Start the Gin server in a goroutine
	go a.startGinServer()
}

// shutdown is called when the app is closing
func (a *App) shutdown(ctx context.Context) {
	// Close database connection
	if err := database.Close(); err != nil {
		fmt.Printf("Failed to close database: %v\n", err)
	}
}

func (a *App) startGinServer() {
	a.router = routes.NewRouter()
	a.router.Setup()
	// ADR-0004 step 2: bind to loopback only. The launcher is single-user and
	// has no legitimate remote caller; listening on all interfaces exposed the
	// API (and its then-unauthenticated routes) to the whole LAN.
	a.router.GetEngine().Run("127.0.0.1:" + a.config.ServerPort)
}

// Greet returns a greeting for the given name
func (a *App) Greet(name string) string {
	return fmt.Sprintf("Hello %s, It's show time!", name)
}

// TestHealthCheck calls the backend health check API
func (a *App) TestHealthCheck() *models.ApiResponse {
	return a.httpClient.Get("/")
}

// TestPing calls the backend ping API
func (a *App) TestPing() *models.ApiResponse {
	return a.httpClient.Get("/ping")
}

// TestAPICall makes a generic API call
func (a *App) TestAPICall(endpoint string) *models.ApiResponse {
	start := time.Now()
	resp, err := http.Get("http://localhost:" + a.config.ServerPort + endpoint)
	if err != nil {
		return &models.ApiResponse{
			Success: false,
			Error:   err.Error(),
			Time:    time.Since(start).Milliseconds(),
		}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &models.ApiResponse{
			Success: false,
			Error:   err.Error(),
			Time:    time.Since(start).Milliseconds(),
		}
	}

	return &models.ApiResponse{
		Success: resp.StatusCode >= 200 && resp.StatusCode < 300,
		Data:    string(body),
		Time:    time.Since(start).Milliseconds(),
	}
}

// GetDBPath returns the database path
func (a *App) GetDBPath() string {
	return database.DefaultConfig().Path
}

// GetDBSchema calls the backend API to get the database schema
func (a *App) GetDBSchema() *models.ApiResponse {
	return a.httpClient.Get("/api/v1/db/schema")
}
