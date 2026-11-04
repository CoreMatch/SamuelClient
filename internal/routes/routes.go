package routes

import (
	"github.com/gin-gonic/gin"

	"HRPMLW/internal/handlers"
	"HRPMLW/internal/middleware"
)

// Router holds the gin engine and handlers
type Router struct {
	Engine  *gin.Engine
	Handler *handlers.Handler
	DB      *handlers.DBHandler
	MC      *handlers.MCHandler
}

// NewRouter creates a new Router
func NewRouter() *Router {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()

	// Apply global middleware
	engine.Use(middleware.Recovery())
	engine.Use(middleware.Logger())
	engine.Use(middleware.CORS())

	return &Router{
		Engine:  engine,
		Handler: handlers.NewHandler(),
		DB:      handlers.NewDBHandler(),
		MC:      handlers.NewMCHandler(),
	}
}

// Setup sets up all routes
func (r *Router) Setup() {
	// API v1 routes
	v1 := r.Engine.Group("/api/v1")
	{
		// Health routes
		v1.GET("/health", r.Handler.Health)
		v1.GET("/ping", r.Handler.Ping)
		v1.GET("/hello/:name", r.Handler.Hello)

		// Utility routes
		v1.POST("/echo", r.Handler.Echo)
		v1.GET("/sysinfo", r.Handler.SysInfo)

		// Database routes
		r.setupDatabaseRoutes(v1)

		// Minecraft launcher routes
		r.setupMCRoutes(v1)
	}

	// Legacy routes (for backward compatibility)
	r.setupLegacyRoutes()

	// 404 handler
	r.Engine.NoRoute(r.Handler.NotFound)
}

// setupDatabaseRoutes sets up database CRUD routes
func (r *Router) setupDatabaseRoutes(rg *gin.RouterGroup) {
	db := rg.Group("/db")
	{
		// Database stats, cleanup, and schema
		db.GET("/stats", r.DB.GetDBStats)
		db.POST("/cleanup", r.DB.CleanupDB)
		db.GET("/schema", r.DB.GetDBSchema)

		// User routes
		users := db.Group("/users")
		{
			users.GET("", r.DB.GetAllUsers)
			users.POST("", r.DB.CreateUser)
			users.GET("/:id", r.DB.GetUser)
			users.PUT("/:id", r.DB.UpdateUser)
			users.DELETE("/:id", r.DB.DeleteUser)
		}

		// Setting routes
		settings := db.Group("/settings")
		{
			settings.GET("", r.DB.GetAllSettings)
			settings.POST("", r.DB.SetSetting)
			settings.GET("/:key", r.DB.GetSetting)
			settings.DELETE("/:key", r.DB.DeleteSetting)
		}

		// Log routes
		logs := db.Group("/logs")
		{
			logs.GET("", r.DB.GetLogs)
			logs.POST("", r.DB.CreateLog)
			logs.DELETE("/old", r.DB.DeleteOldLogs)
		}

		// Task routes
		tasks := db.Group("/tasks")
		{
			tasks.GET("", r.DB.GetTasks)
			tasks.POST("", r.DB.CreateTask)
			tasks.GET("/:id", r.DB.GetTask)
			tasks.PUT("/:id", r.DB.UpdateTask)
			tasks.PATCH("/:id/status", r.DB.UpdateTaskStatus)
			tasks.PATCH("/:id/progress", r.DB.UpdateTaskProgress)
			tasks.DELETE("/:id", r.DB.DeleteTask)
		}

		// File record routes
		files := db.Group("/files")
		{
			files.GET("", r.DB.GetAllFileRecords)
			files.POST("", r.DB.CreateFileRecord)
			files.GET("/:id", r.DB.GetFileRecord)
			files.PUT("/:id", r.DB.UpdateFileRecord)
			files.DELETE("/:id", r.DB.DeleteFileRecord)
		}

		// Java installation routes
		java := db.Group("/java")
		{
			java.GET("", r.DB.GetAllJavaInstallations)
			java.POST("", r.DB.CreateJavaInstallation)
			java.GET("/:id", r.DB.GetJavaInstallation)
			java.PUT("/:id", r.DB.UpdateJavaInstallation)
			java.POST("/:id/default", r.DB.SetDefaultJavaInstallation)
			java.DELETE("/:id", r.DB.DeleteJavaInstallation)
			java.GET("/default", r.DB.GetDefaultJavaInstallation)
		}
	}
}

// setupMCRoutes sets up Minecraft launcher routes
func (r *Router) setupMCRoutes(rg *gin.RouterGroup) {
	// Auth routes
	auth := rg.Group("/auth")
	{
		auth.GET("/meta", r.MC.GetAuthlibMeta)
		auth.POST("/login", r.MC.AuthlibLogin)
		auth.POST("/refresh", r.MC.AuthlibRefresh)
		auth.POST("/validate", r.MC.AuthlibValidate)
		auth.POST("/offline", r.MC.OfflineLogin)
	}

	// Version routes
	versions := rg.Group("/versions")
	{
		versions.GET("", r.MC.GetVersions)
		versions.GET("/:id", r.MC.GetVersionManifest)
	}

	// Fabric routes
	fabric := rg.Group("/fabric")
	{
		fabric.GET("/versions", r.MC.GetFabricVersions)
	}

	// Forge routes
	forge := rg.Group("/forge")
	{
		forge.GET("/versions", r.MC.GetForgeVersions)
	}

	// Download routes
	download := rg.Group("/download")
	{
		download.POST("/version", r.MC.DownloadVersion)
	}

	// Launcher routes
	launcher := rg.Group("/launcher")
	{
		launcher.GET("/config", r.MC.GetLauncherConfig)
		launcher.PUT("/config", r.MC.UpdateLauncherConfig)
		launcher.POST("/launch", r.MC.LaunchGame)
		launcher.GET("/minecraft-dir", r.MC.GetMinecraftDir)
		launcher.GET("/installed-versions", r.MC.ListInstalledVersions)
	}
}

// setupLegacyRoutes sets up legacy routes for backward compatibility
func (r *Router) setupLegacyRoutes() {
	r.Engine.GET("/", r.Handler.Health)
	r.Engine.GET("/ping", r.Handler.Ping)
	r.Engine.GET("/hello/:name", r.Handler.Hello)
	r.Engine.POST("/echo", r.Handler.Echo)
	r.Engine.GET("/sysinfo", r.Handler.SysInfo)
}

// SetupWithGroup sets up routes with a custom group prefix
func (r *Router) SetupWithGroup(prefix string) {
	group := r.Engine.Group(prefix)
	{
		group.GET("/health", r.Handler.Health)
		group.GET("/ping", r.Handler.Ping)
		group.GET("/hello/:name", r.Handler.Hello)
		group.POST("/echo", r.Handler.Echo)
		group.GET("/sysinfo", r.Handler.SysInfo)
	}
}

// GetEngine returns the gin engine
func (r *Router) GetEngine() *gin.Engine {
	return r.Engine
}
