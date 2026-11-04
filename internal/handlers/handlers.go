package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"HRPMLW/internal/models"
	"HRPMLW/internal/services"
)

// Handler holds all handlers
type Handler struct {
	sysSvc *services.SysService
}

// NewHandler creates a new Handler
func NewHandler() *Handler {
	return &Handler{
		sysSvc: services.NewSysService(),
	}
}

// Health handles GET / - health check
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, models.HealthResponse{
		Status:  "ok",
		Message: "Samuel Client is running",
	})
}

// Ping handles GET /ping
func (h *Handler) Ping(c *gin.Context) {
	c.JSON(http.StatusOK, models.PingResponse{
		Message: "pong",
	})
}

// Hello handles GET /hello/:name
func (h *Handler) Hello(c *gin.Context) {
	name := c.Param("name")
	c.JSON(http.StatusOK, models.HelloResponse{
		Message: "Hello " + name,
	})
}

// Echo handles POST /echo
func (h *Handler) Echo(c *gin.Context) {
	var requestBody map[string]interface{}
	if err := c.BindJSON(&requestBody); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, requestBody)
}

// SysInfo handles GET /sysinfo
func (h *Handler) SysInfo(c *gin.Context) {
	info := h.sysSvc.GetInfo()
	c.JSON(http.StatusOK, info)
}

// NotFound handles 404
func (h *Handler) NotFound(c *gin.Context) {
	c.JSON(http.StatusNotFound, models.ErrorResponse{
		Error: fmt.Sprintf("route %s not found", c.Request.URL.Path),
	})
}
