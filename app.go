package main

import (
	"HRPMLW/internal/database"
	"HRPMLW/internal/models"
	"HRPMLW/internal/repository"
	"HRPMLW/internal/services"
	"context"
	"fmt"
)

// App struct
//
// ADR-0004: the frontend reaches the backend exclusively through the Wails
// binding methods in bindings.go. There is no local HTTP listener; the
// previous embedded Gin server, its routes/middleware, the HTTP client, and
// the SERVER_PORT configuration have all been retired.
type App struct {
	ctx context.Context

	authService     *services.AuthService
	bmclService     *services.BMCLAPIService
	launcherService *services.LauncherService
	javaRepo        *repository.JavaRepository
	logRepo         *repository.LogRepository
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		authService:     services.NewAuthService(),
		bmclService:     services.NewBMCLAPIService(),
		launcherService: services.NewLauncherService(),
		javaRepo:        repository.NewJavaRepository(),
		logRepo:         repository.NewLogRepository(),
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
}

// shutdown is called when the app is closing
func (a *App) shutdown(ctx context.Context) {
	// Close database connection
	if err := database.Close(); err != nil {
		fmt.Printf("Failed to close database: %v\n", err)
	}
}

// GetDBPath returns the database path
func (a *App) GetDBPath() string {
	return database.DefaultConfig().Path
}

// GetDBSchema returns the SQLite schema directly (no HTTP hop, ADR-0004).
func (a *App) GetDBSchema() *models.ApiResponse {
	schema, err := database.GetSchema()
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	return &models.ApiResponse{Success: true, Data: schema}
}
