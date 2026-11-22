package services

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"path/filepath"
	"strings"

	"HRPMLW/internal/models"
)

type textureAPIEnvelope struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Error   string          `json:"error"`
	Data    json.RawMessage `json:"data"`
}

// GetHRPAuthTextures returns the authenticated account's current profile
// texture bindings from the Yggdrasil business API.
func (a *AuthService) GetHRPAuthTextures(yggdrasilAccessToken string) *models.ApiResponse {
	if yggdrasilAccessToken == "" {
		return &models.ApiResponse{Success: false, Error: "Yggdrasil access token is required"}
	}
	baseURL := strings.TrimSuffix(HRPAuthBaseURL, "/")
	req, err := http.NewRequest(http.MethodPost, baseURL+"/texture/get", nil)
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+yggdrasilAccessToken)
	req.Header.Set("Accept", "application/json")
	result := a.doTextureRequest(req)
	if data, ok := result.Data.(map[string]interface{}); result.Success && ok {
		data["textureDownloadBaseUrl"] = baseURL + "/textures/"
	}
	return result
}

// UploadHRPAuthTexture uploads and activates a PNG texture for a profile.
func (a *AuthService) UploadHRPAuthTexture(yggdrasilAccessToken, profileID, textureType, model, fileName string, imageData []byte) *models.ApiResponse {
	if yggdrasilAccessToken == "" || !validYggdrasilProfileID(profileID) {
		return &models.ApiResponse{Success: false, Error: "Yggdrasil access token and a valid profile id are required"}
	}
	if textureType != "skin" && textureType != "cape" {
		return &models.ApiResponse{Success: false, Error: "texture type must be skin or cape"}
	}
	if len(imageData) == 0 || len(imageData) > 2*1024*1024 {
		return &models.ApiResponse{Success: false, Error: "PNG file must be between 1 byte and 2 MiB"}
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("type", textureType); err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	if textureType == "skin" && model != "" {
		if model != "default" && model != "slim" {
			return &models.ApiResponse{Success: false, Error: "skin model must be default or slim"}
		}
		if err := writer.WriteField("model", model); err != nil {
			return &models.ApiResponse{Success: false, Error: err.Error()}
		}
	}

	if fileName == "" {
		fileName = "texture.png"
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{
		"name": "file", "filename": filepath.Base(fileName),
	}))
	header.Set("Content-Type", "image/png")
	part, err := writer.CreatePart(header)
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	if _, err := part.Write(imageData); err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	if err := writer.Close(); err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}

	url := strings.TrimSuffix(HRPAuthBaseURL, "/") + "/texture/upload/" + profileID
	req, err := http.NewRequest(http.MethodPost, url, &body)
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+yggdrasilAccessToken)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	return a.doTextureRequest(req)
}

// DeleteHRPAuthTexture removes the active skin or cape binding from a profile.
func (a *AuthService) DeleteHRPAuthTexture(yggdrasilAccessToken, profileID, textureType string) *models.ApiResponse {
	if yggdrasilAccessToken == "" || !validYggdrasilProfileID(profileID) {
		return &models.ApiResponse{Success: false, Error: "Yggdrasil access token and a valid profile id are required"}
	}
	if textureType != "skin" && textureType != "cape" {
		return &models.ApiResponse{Success: false, Error: "texture type must be skin or cape"}
	}
	url := strings.TrimSuffix(HRPAuthBaseURL, "/") + "/texture/delete/" + profileID + "/" + textureType
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return &models.ApiResponse{Success: false, Error: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+yggdrasilAccessToken)
	req.Header.Set("Accept", "application/json")
	return a.doTextureRequest(req)
}

func validYggdrasilProfileID(profileID string) bool {
	normalized := strings.ReplaceAll(profileID, "-", "")
	if len(normalized) != 32 {
		return false
	}
	_, err := hex.DecodeString(normalized)
	return err == nil
}

func (a *AuthService) doTextureRequest(req *http.Request) *models.ApiResponse {
	resp, err := a.client.Do(req)
	if err != nil {
		return &models.ApiResponse{Success: false, Error: "Yggdrasil API request failed: " + err.Error()}
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return &models.ApiResponse{Success: false, Error: "read Yggdrasil API response: " + err.Error()}
	}
	var envelope textureAPIEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return &models.ApiResponse{Success: false, Error: fmt.Sprintf("Yggdrasil API returned an invalid response (HTTP %d)", resp.StatusCode)}
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices || !envelope.Success {
		message := envelope.Message
		if message == "" {
			message = envelope.Error
		}
		if message == "" {
			message = fmt.Sprintf("Yggdrasil API request failed (HTTP %d)", resp.StatusCode)
		}
		return &models.ApiResponse{Success: false, Error: message}
	}

	var data interface{}
	if len(envelope.Data) > 0 && string(envelope.Data) != "null" {
		if err := json.Unmarshal(envelope.Data, &data); err != nil {
			return &models.ApiResponse{Success: false, Error: "parse Yggdrasil API response: " + err.Error()}
		}
	}
	return &models.ApiResponse{Success: true, Data: data}
}
