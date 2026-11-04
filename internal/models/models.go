package models

// ApiResponse represents the standard response structure returned by every
// Wails binding method (ADR-0004). The frontend reads { success, data, error }.
type ApiResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
	Error   string      `json:"error,omitempty"`
	Time    int64       `json:"time,omitempty"`
}
