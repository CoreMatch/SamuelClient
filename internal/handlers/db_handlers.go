package handlers

import (
	"HRPMLW/internal/database"
	"HRPMLW/internal/models"
	"HRPMLW/internal/repository"
	"HRPMLW/internal/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// DBHandler holds database handlers
type DBHandler struct {
	userRepo    *repository.UserRepository
	settingRepo *repository.SettingRepository
	logRepo     *repository.LogRepository
	taskRepo    *repository.TaskRepository
	fileRepo    *repository.FileRepository
	javaRepo    *repository.JavaRepository
}

// NewDBHandler creates a new DBHandler
func NewDBHandler() *DBHandler {
	return &DBHandler{
		userRepo:    repository.NewUserRepository(),
		settingRepo: repository.NewSettingRepository(),
		logRepo:     repository.NewLogRepository(),
		taskRepo:    repository.NewTaskRepository(),
		fileRepo:    repository.NewFileRepository(),
		javaRepo:    repository.NewJavaRepository(),
	}
}

// ========== User Handlers ==========

// CreateUser handles POST /db/users
func (h *DBHandler) CreateUser(c *gin.Context) {
	var user models.User
	if err := c.BindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	if user.Username == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "username is required"})
		return
	}

	if user.Role == "" {
		user.Role = models.RoleUser
	}

	if err := h.userRepo.Create(&user); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusCreated, user)
}

// GetUser handles GET /db/users/:id
func (h *DBHandler) GetUser(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid user id"})
		return
	}

	user, err := h.userRepo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, user)
}

// GetAllUsers handles GET /db/users
func (h *DBHandler) GetAllUsers(c *gin.Context) {
	users, err := h.userRepo.GetAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, users)
}

// UpdateUser handles PUT /db/users/:id
func (h *DBHandler) UpdateUser(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid user id"})
		return
	}

	var user models.User
	if err := c.BindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	user.ID = id
	if err := h.userRepo.Update(&user); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, user)
}

// DeleteUser handles DELETE /db/users/:id
func (h *DBHandler) DeleteUser(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid user id"})
		return
	}

	if err := h.userRepo.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "user deleted"})
}

// ========== Setting Handlers ==========

// SetSetting handles POST /db/settings
func (h *DBHandler) SetSetting(c *gin.Context) {
	var req struct {
		Key         string `json:"key"`
		Value       string `json:"value"`
		Type        string `json:"type"`
		Description string `json:"description"`
	}

	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	if req.Key == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "key is required"})
		return
	}

	if req.Type == "" {
		req.Type = "string"
	}

	if err := h.settingRepo.Set(req.Key, req.Value, req.Type, req.Description); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	setting, _ := h.settingRepo.Get(req.Key)
	c.JSON(http.StatusOK, setting)
}

// GetSetting handles GET /db/settings/:key
func (h *DBHandler) GetSetting(c *gin.Context) {
	key := c.Param("key")

	setting, err := h.settingRepo.Get(key)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, setting)
}

// GetAllSettings handles GET /db/settings
func (h *DBHandler) GetAllSettings(c *gin.Context) {
	settings, err := h.settingRepo.GetAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, settings)
}

// DeleteSetting handles DELETE /db/settings/:key
func (h *DBHandler) DeleteSetting(c *gin.Context) {
	key := c.Param("key")

	if err := h.settingRepo.Delete(key); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "setting deleted"})
}

// ========== Log Handlers ==========

// CreateLog handles POST /db/logs
func (h *DBHandler) CreateLog(c *gin.Context) {
	var log models.Log
	if err := c.BindJSON(&log); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	if log.Message == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "message is required"})
		return
	}

	if log.Level == "" {
		log.Level = models.LogLevelInfo
	}

	if err := h.logRepo.Create(&log); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusCreated, log)
}

// GetLogs handles GET /db/logs
func (h *DBHandler) GetLogs(c *gin.Context) {
	level := c.Query("level")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	logs, err := h.logRepo.GetAll(level, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, logs)
}

// DeleteOldLogs handles DELETE /db/logs/old
func (h *DBHandler) DeleteOldLogs(c *gin.Context) {
	days, err := strconv.Atoi(c.DefaultQuery("days", "30"))
	if err != nil {
		days = 30
	}

	if err := h.logRepo.DeleteOld(days); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "old logs deleted"})
}

// ========== Task Handlers ==========

// CreateTask handles POST /db/tasks
func (h *DBHandler) CreateTask(c *gin.Context) {
	var task models.Task
	if err := c.BindJSON(&task); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	if task.Name == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "name is required"})
		return
	}

	if task.Status == "" {
		task.Status = models.TaskStatusPending
	}

	if err := h.taskRepo.Create(&task); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusCreated, task)
}

// GetTask handles GET /db/tasks/:id
func (h *DBHandler) GetTask(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid task id"})
		return
	}

	task, err := h.taskRepo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, task)
}

// GetTasks handles GET /db/tasks
func (h *DBHandler) GetTasks(c *gin.Context) {
	status := c.Query("status")

	tasks, err := h.taskRepo.GetAll(status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, tasks)
}

// UpdateTask handles PUT /db/tasks/:id
func (h *DBHandler) UpdateTask(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid task id"})
		return
	}

	var task models.Task
	if err := c.BindJSON(&task); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	task.ID = id
	if err := h.taskRepo.Update(&task); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, task)
}

// UpdateTaskStatus handles PATCH /db/tasks/:id/status
func (h *DBHandler) UpdateTaskStatus(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid task id"})
		return
	}

	var req struct {
		Status string `json:"status"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	if err := h.taskRepo.UpdateStatus(id, req.Status); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "task status updated"})
}

// UpdateTaskProgress handles PATCH /db/tasks/:id/progress
func (h *DBHandler) UpdateTaskProgress(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid task id"})
		return
	}

	var req struct {
		Progress int `json:"progress"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	if err := h.taskRepo.UpdateProgress(id, req.Progress); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "task progress updated"})
}

// DeleteTask handles DELETE /db/tasks/:id
func (h *DBHandler) DeleteTask(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid task id"})
		return
	}

	if err := h.taskRepo.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "task deleted"})
}

// ========== File Handlers ==========

// CreateFileRecord handles POST /db/files
func (h *DBHandler) CreateFileRecord(c *gin.Context) {
	var file models.File
	if err := c.BindJSON(&file); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	if file.Name == "" || file.Path == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "name and path are required"})
		return
	}

	if err := h.fileRepo.Create(&file); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusCreated, file)
}

// GetFileRecord handles GET /db/files/:id
func (h *DBHandler) GetFileRecord(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid file id"})
		return
	}

	file, err := h.fileRepo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, file)
}

// GetAllFileRecords handles GET /db/files
func (h *DBHandler) GetAllFileRecords(c *gin.Context) {
	files, err := h.fileRepo.GetAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, files)
}

// UpdateFileRecord handles PUT /db/files/:id
func (h *DBHandler) UpdateFileRecord(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid file id"})
		return
	}

	var file models.File
	if err := c.BindJSON(&file); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	file.ID = id
	if err := h.fileRepo.Update(&file); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, file)
}

// DeleteFileRecord handles DELETE /db/files/:id
func (h *DBHandler) DeleteFileRecord(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid file id"})
		return
	}

	if err := h.fileRepo.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "file record deleted"})
}

// ========== Database Stats ==========

// GetDBStats handles GET /db/stats
func (h *DBHandler) GetDBStats(c *gin.Context) {
	stats, err := repository.Stats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// CleanupDB handles POST /db/cleanup
func (h *DBHandler) CleanupDB(c *gin.Context) {
	if err := repository.Cleanup(); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "database cleaned up"})
}

// GetDBSchema handles GET /db/schema
func (h *DBHandler) GetDBSchema(c *gin.Context) {
	schema, err := database.GetSchema()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": schema})
}

// ========== Java Installation Handlers ==========

// CreateJavaInstallation handles POST /db/java
func (h *DBHandler) CreateJavaInstallation(c *gin.Context) {
	var req struct {
		Path         string `json:"path"`
		FriendlyName string `json:"friendly_name"`
	}

	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	if req.Path == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "path is required"})
		return
	}

	version, err := services.DetectJavaVersion(req.Path)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "Failed to detect Java version: " + err.Error()})
		return
	}

	java := &models.JavaInstallation{
		Path:         req.Path,
		FriendlyName: req.FriendlyName,
		Version:      version,
		IsDefault:    false,
	}

	if err := h.javaRepo.Create(java); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusCreated, java)
}

// GetJavaInstallation handles GET /db/java/:id
func (h *DBHandler) GetJavaInstallation(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid java installation id"})
		return
	}

	java, err := h.javaRepo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, java)
}

// GetAllJavaInstallations handles GET /db/java
func (h *DBHandler) GetAllJavaInstallations(c *gin.Context) {
	javas, err := h.javaRepo.GetAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	// If no Java installations exist, auto-detect system Java
	if len(javas) == 0 {
		systemJavaPath := services.DetectSystemJava()
		if systemJavaPath != "" {
			version, _ := services.DetectJavaVersion(systemJavaPath)
			defaultJava := &models.JavaInstallation{
				Path:         systemJavaPath,
				FriendlyName: "System Default Java",
				Version:      version,
				IsDefault:    true,
			}
			if err := h.javaRepo.Create(defaultJava); err == nil {
				javas = []models.JavaInstallation{*defaultJava}
			}
		}
	}

	c.JSON(http.StatusOK, javas)
}

// GetDefaultJavaInstallation handles GET /db/java/default
func (h *DBHandler) GetDefaultJavaInstallation(c *gin.Context) {
	java, err := h.javaRepo.GetDefault()
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, java)
}

// UpdateJavaInstallation handles PUT /db/java/:id
func (h *DBHandler) UpdateJavaInstallation(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid java installation id"})
		return
	}

	var req struct {
		Path         string `json:"path"`
		FriendlyName string `json:"friendly_name"`
	}

	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}

	java, err := h.javaRepo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{Error: err.Error()})
		return
	}

	if req.Path != "" {
		java.Path = req.Path
		version, err := services.DetectJavaVersion(req.Path)
		if err != nil {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "Failed to detect Java version: " + err.Error()})
			return
		}
		java.Version = version
	}

	if req.FriendlyName != "" {
		java.FriendlyName = req.FriendlyName
	}

	if err := h.javaRepo.Update(java); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, java)
}

// SetDefaultJavaInstallation handles POST /db/java/:id/default
func (h *DBHandler) SetDefaultJavaInstallation(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid java installation id"})
		return
	}

	if err := h.javaRepo.SetDefault(id); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "java installation set as default"})
}

// DeleteJavaInstallation handles DELETE /db/java/:id
func (h *DBHandler) DeleteJavaInstallation(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "invalid java installation id"})
		return
	}

	if err := h.javaRepo.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "java installation deleted"})
}
