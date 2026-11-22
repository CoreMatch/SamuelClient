package services

import (
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetHRPAuthTexturesUsesYggdrasilSessionToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/texture/get" {
			t.Errorf("request = %s %s, want POST /texture/get", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer yggdrasil-session-token" {
			t.Errorf("Authorization = %q", got)
		}
		_, _ = io.WriteString(w, `{"success":true,"message":"Success","data":{"profiles":[{"id":"0123456789abcdef0123456789abcdef","textures":{"skin":{"url":"https://ha.mcnb.dev/textures/skin"}}}]}}`)
	}))
	defer server.Close()
	pointHRPAuthAt(t, server.URL)

	result := NewAuthService().GetHRPAuthTextures("yggdrasil-session-token")
	if !result.Success {
		t.Fatalf("GetHRPAuthTextures failed: %s", result.Error)
	}
	data, ok := result.Data.(map[string]interface{})
	if !ok || data["profiles"] == nil {
		t.Fatalf("Data = %#v, want profiles", result.Data)
	}
}

func TestUploadHRPAuthTexture(t *testing.T) {
	imageData := []byte{0x89, 'P', 'N', 'G'}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/texture/upload/0123456789abcdef0123456789abcdef" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer session-token" {
			t.Errorf("Authorization = %q", got)
		}
		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "multipart/form-data" {
			t.Fatalf("Content-Type = %q, err = %v", r.Header.Get("Content-Type"), err)
		}
		reader := multipart.NewReader(r.Body, params["boundary"])
		fields := map[string]string{}
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			data, _ := io.ReadAll(part)
			if part.FormName() == "file" {
				if part.FileName() != "skin.png" || string(data) != string(imageData) {
					t.Errorf("file part = %q, %q", part.FileName(), data)
				}
				if part.Header.Get("Content-Type") != "image/png" {
					t.Errorf("file Content-Type = %q", part.Header.Get("Content-Type"))
				}
			} else {
				fields[part.FormName()] = string(data)
			}
		}
		if fields["type"] != "skin" || fields["model"] != "slim" {
			t.Errorf("multipart fields = %#v", fields)
		}
		_, _ = io.WriteString(w, `{"success":true,"message":"Texture uploaded","data":{"warnings":[]}}`)
	}))
	defer server.Close()
	pointHRPAuthAt(t, server.URL)

	result := NewAuthService().UploadHRPAuthTexture(
		"session-token",
		"0123456789abcdef0123456789abcdef",
		"skin",
		"slim",
		"skin.png",
		imageData,
	)
	if !result.Success {
		t.Fatalf("UploadHRPAuthTexture failed: %s", result.Error)
	}
}

func TestDeleteHRPAuthTextureAndInvalidType(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodDelete || r.URL.Path != "/texture/delete/0123456789abcdef0123456789abcdef/cape" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer session-token" {
			t.Errorf("Authorization = %q", got)
		}
		_, _ = io.WriteString(w, `{"success":true,"message":"Texture deleted"}`)
	}))
	defer server.Close()
	pointHRPAuthAt(t, server.URL)

	service := NewAuthService()
	result := service.DeleteHRPAuthTexture("session-token", "0123456789abcdef0123456789abcdef", "cape")
	if !result.Success {
		t.Fatalf("DeleteHRPAuthTexture failed: %s", result.Error)
	}
	if invalid := service.DeleteHRPAuthTexture("session-token", "0123456789abcdef0123456789abcdef", "hat"); invalid.Success || !strings.Contains(invalid.Error, "skin or cape") {
		t.Fatalf("invalid type response = %+v", invalid)
	}
	if requests != 1 {
		t.Errorf("server requests = %d, want 1", requests)
	}
}

func TestTextureAPIErrorMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"success":false,"message":"Unauthorized"}`)
	}))
	defer server.Close()
	pointHRPAuthAt(t, server.URL)

	result := NewAuthService().GetHRPAuthTextures("expired-token")
	if result.Success || result.Error != "Unauthorized" {
		t.Fatalf("response = %+v, want Unauthorized failure", result)
	}
}
