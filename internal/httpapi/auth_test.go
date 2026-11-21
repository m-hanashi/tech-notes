package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testAPIKey = "test-key"

// newTestHandler は、本番と同じ入口（認証つき）を、偽の保存先で作る。
func newTestHandler(t *testing.T, store NoteStore) http.Handler {
	t.Helper()
	auth, err := NewAPIKeyAuthenticator(testAPIKey)
	if err != nil {
		t.Fatal(err)
	}
	return NewHandler(store, auth)
}

func TestAuth(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		apiKey     string // 空なら、X-API-Key を付けない
		wantStatus int
	}{
		{"正しいキーなら、メモの API が動く", http.MethodGet, "/notes", testAPIKey, http.StatusOK},
		{"キーがないと 401（一覧）", http.MethodGet, "/notes", "", http.StatusUnauthorized},
		{"キーがないと 401（登録）", http.MethodPost, "/notes", "", http.StatusUnauthorized},
		{"キーがないと 401（取得）", http.MethodGet, "/notes/0193a2f4-7c1e-7b3a-9f21-4c8d2e6b1a05", "", http.StatusUnauthorized},
		{"キーが違うと 401", http.MethodGet, "/notes", "wrong-key", http.StatusUnauthorized},
		{"キーがなければ、存在しないパスも 401", http.MethodGet, "/xyz", "", http.StatusUnauthorized},
		{"/health はキーなしで 200", http.MethodGet, "/health", "", http.StatusOK},
		{"//health もキーなしで通し、ルーターが /health に案内する", http.MethodGet, "//health", "", http.StatusTemporaryRedirect},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeStore{}
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(`{"title":"題名","body":"本文"}`))
			if tt.apiKey != "" {
				req.Header.Set("X-API-Key", tt.apiKey)
			}
			rec := httptest.NewRecorder()
			newTestHandler(t, store).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("状態コード：got %d, want %d（本文：%s）", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantStatus == http.StatusUnauthorized {
				assertProblem(t, rec, http.StatusUnauthorized)
				if got := rec.Header().Get("WWW-Authenticate"); got == "" {
					t.Error("401 に WWW-Authenticate がない")
				}
				if store.calls != 0 {
					t.Errorf("断ったのに、保存先が %d 回呼ばれた", store.calls)
				}
			}
		})
	}
}

func TestNewAPIKeyAuthenticator_RejectsEmptyKey(t *testing.T) {
	if _, err := NewAPIKeyAuthenticator(""); err == nil {
		t.Error("空のキーで、エラーにならなかった")
	}
}

func TestNewHandler_PanicsWithoutAuthenticator(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("認証の部品が nil なのに、止まらなかった")
		}
	}()
	NewHandler(&fakeStore{}, nil)
}
