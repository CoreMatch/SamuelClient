package models

import "time"

// ApiResponse represents the standard API response structure
type ApiResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
	Error   string      `json:"error,omitempty"`
	Time    int64       `json:"time,omitempty"`
}

// HealthResponse represents health check response
type HealthResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// PingResponse represents ping response
type PingResponse struct {
	Message string `json:"message"`
}

// HelloResponse represents hello response
type HelloResponse struct {
	Message string `json:"message"`
}

// SysInfoResponse represents system info response
type SysInfoResponse struct {
	Arch string `json:"arch"`
	OS   string `json:"os"`
}

// ErrorResponse represents error response
type ErrorResponse struct {
	Error string `json:"error"`
}

// NewApiResponse creates a new API response
func NewApiResponse(success bool, data interface{}, err string) *ApiResponse {
	return &ApiResponse{
		Success: success,
		Data:    data,
		Error:   err,
		Time:    time.Now().UnixMilli(),
	}
}
