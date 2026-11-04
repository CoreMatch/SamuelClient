package services

import (
	"encoding/json"
	"io"
	"net/http"
	"runtime"
	"time"

	"HRPMLW/internal/models"
)

// SysService handles system information
type SysService struct{}

// NewSysService creates a new SysService
func NewSysService() *SysService {
	return &SysService{}
}

// GetInfo returns system information
func (s *SysService) GetInfo() *models.SysInfoResponse {
	return &models.SysInfoResponse{
		Arch: runtime.GOARCH,
		OS:   runtime.GOOS,
	}
}

// HTTPService handles HTTP requests
type HTTPService struct {
	baseURL string
}

// NewHTTPService creates a new HTTPService
func NewHTTPService(baseURL string) *HTTPService {
	return &HTTPService{baseURL: baseURL}
}

// Get performs a GET request
func (h *HTTPService) Get(endpoint string) *models.ApiResponse {
	start := time.Now()
	resp, err := http.Get(h.baseURL + endpoint)
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

	var data interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return &models.ApiResponse{
			Success: false,
			Error:   err.Error(),
			Time:    time.Since(start).Milliseconds(),
		}
	}

	return &models.ApiResponse{
		Success: resp.StatusCode >= 200 && resp.StatusCode < 300,
		Data:    data,
		Time:    time.Since(start).Milliseconds(),
	}
}
