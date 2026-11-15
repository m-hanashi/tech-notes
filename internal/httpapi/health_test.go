package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealth(t *testing.T) {
	tests := []struct {
		name            string
		method          string
		path            string
		wantStatus      int
		wantContentType string // 空なら確かめない
		wantBody        string // 空なら確かめない
	}{
		{
			name:            "GET /health は 200 と JSON の本文を返す",
			method:          http.MethodGet,
			path:            "/health",
			wantStatus:      http.StatusOK,
			wantContentType: "application/json",
			wantBody:        `{"status":"ok"}`,
		},
		{
			name:       "POST /health は 405 を返す",
			method:     http.MethodPost,
			path:       "/health",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "GET /healthz は 404 を返す",
			method:     http.MethodGet,
			path:       "/healthz",
			wantStatus: http.StatusNotFound,
		},
	}

	router := NewRouter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("状態コード：got %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantContentType != "" {
				if got := rec.Header().Get("Content-Type"); got != tt.wantContentType {
					t.Errorf("Content-Type：got %q, want %q", got, tt.wantContentType)
				}
			}
			if tt.wantBody != "" {
				if got := rec.Body.String(); got != tt.wantBody {
					t.Errorf("本文：got %q, want %q", got, tt.wantBody)
				}
			}
		})
	}
}
