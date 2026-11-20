package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/m-hanashi/tech-notes/internal/notes"
)

// fakeStore は、テスト用の偽の保存先。保存したメモを覚え、決めたエラーを返せる。
type fakeStore struct {
	created []notes.Note
	err     error
}

func (s *fakeStore) Create(_ context.Context, n notes.Note) error {
	if s.err != nil {
		return s.err
	}
	s.created = append(s.created, n)
	return nil
}

func (s *fakeStore) Get(_ context.Context, id uuid.UUID) (notes.Note, error) {
	if s.err != nil {
		return notes.Note{}, s.err
	}
	for _, n := range s.created {
		if n.ID == id {
			return n, nil
		}
	}
	return notes.Note{}, notes.ErrNotFound
}

func postNote(t *testing.T, store NoteStore, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/notes", strings.NewReader(body))
	rec := httptest.NewRecorder()
	NewRouter(store).ServeHTTP(rec, req)
	return rec
}

// jsonBody は、title と body から、リクエストの本文の JSON を作る。
func jsonBody(title, body string) string {
	b, _ := json.Marshal(map[string]string{"title": title, "body": body})
	return string(b)
}

// ① 正しい使い方
func TestCreateNote_Success(t *testing.T) {
	store := &fakeStore{}
	rec := postNote(t, store, jsonBody("買い物", "牛乳とパンを買う"))

	if rec.Code != http.StatusCreated {
		t.Fatalf("状態コード：got %d, want 201（本文：%s）", rec.Code, rec.Body.String())
	}

	var got notes.Note
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("本文を読めない：%v", err)
	}
	if got.Title != "買い物" || got.Body != "牛乳とパンを買う" {
		t.Errorf("title と body：got %q, %q", got.Title, got.Body)
	}
	if got.CreatedAt.IsZero() {
		t.Error("created_at が空")
	}
	if got.ID.Version() != 7 {
		t.Errorf("id の種類：got v%d, want v7", got.ID.Version())
	}
	if loc := rec.Header().Get("Location"); loc != "/notes/"+got.ID.String() {
		t.Errorf("Location：got %q", loc)
	}
	if len(store.created) != 1 || store.created[0].ID != got.ID {
		t.Errorf("保存したメモが、返したメモと一致しない：%+v", store.created)
	}
}

// ② 間違った使い方と、③ 境目の値
func TestCreateNote_Validation(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{"title がない", `{"body":"本文"}`, http.StatusBadRequest},
		{"title が空", jsonBody("", "本文"), http.StatusBadRequest},
		{"title が空白だけ", jsonBody("   ", "本文"), http.StatusBadRequest},
		{"body がない", `{"title":"題名"}`, http.StatusBadRequest},
		{"body が空", jsonBody("題名", ""), http.StatusBadRequest},
		{"body が空白だけ", jsonBody("題名", "   "), http.StatusBadRequest},
		{"JSON が壊れている", `{"title":`, http.StatusBadRequest},
		{"知らない項目がある", `{"title":"題名","body":"本文","tag":"x"}`, http.StatusBadRequest},
		{"本文が1MBを超える", jsonBody("題名", strings.Repeat("a", 1<<20)), http.StatusRequestEntityTooLarge},
		{"title が200文字", jsonBody(strings.Repeat("a", 200), "本文"), http.StatusCreated},
		{"title が201文字", jsonBody(strings.Repeat("a", 201), "本文"), http.StatusBadRequest},
		{"title が日本語200文字", jsonBody(strings.Repeat("あ", 200), "本文"), http.StatusCreated},
		{"body が10,000文字", jsonBody("題名", strings.Repeat("a", 10000)), http.StatusCreated},
		{"body が10,001文字", jsonBody("題名", strings.Repeat("a", 10001)), http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postNote(t, &fakeStore{}, tt.body)
			if rec.Code != tt.wantStatus {
				t.Fatalf("状態コード：got %d, want %d（本文：%s）", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantStatus >= 400 {
				assertProblem(t, rec, tt.wantStatus)
			}
		})
	}
}

// ④ ほかのものの不調
func TestCreateNote_StoreError(t *testing.T) {
	store := &fakeStore{err: errors.New("connection refused: 10.0.1.23:5432")}
	rec := postNote(t, store, jsonBody("題名", "本文"))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("状態コード：got %d, want 500", rec.Code)
	}
	assertProblem(t, rec, http.StatusInternalServerError)
	if strings.Contains(rec.Body.String(), "10.0.1.23") {
		t.Errorf("DB のエラー文が、応答に含まれている：%s", rec.Body.String())
	}
}

func getNote(t *testing.T, store NoteStore, id string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/notes/"+id, nil)
	rec := httptest.NewRecorder()
	NewRouter(store).ServeHTTP(rec, req)
	return rec
}

func TestGetNote(t *testing.T) {
	saved := notes.Note{
		ID:        uuid.Must(uuid.NewV7()),
		Title:     "買い物",
		Body:      "牛乳を買う",
		CreatedAt: time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC),
	}

	t.Run("ある id は 200 とメモを返す", func(t *testing.T) {
		rec := getNote(t, &fakeStore{created: []notes.Note{saved}}, saved.ID.String())
		if rec.Code != http.StatusOK {
			t.Fatalf("状態コード：got %d, want 200（本文：%s）", rec.Code, rec.Body.String())
		}
		var got notes.Note
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("本文を読めない：%v", err)
		}
		if got != saved {
			t.Errorf("返したメモ：got %+v, want %+v", got, saved)
		}
	})

	t.Run("ない id は 404 を返す", func(t *testing.T) {
		rec := getNote(t, &fakeStore{}, uuid.Must(uuid.NewV7()).String())
		if rec.Code != http.StatusNotFound {
			t.Fatalf("状態コード：got %d, want 404", rec.Code)
		}
		assertProblem(t, rec, http.StatusNotFound)
	})

	t.Run("UUID の形でない id は 400 を返す", func(t *testing.T) {
		rec := getNote(t, &fakeStore{}, "abc")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("状態コード：got %d, want 400", rec.Code)
		}
		assertProblem(t, rec, http.StatusBadRequest)
	})

	t.Run("DB の不調は 500 を返し、エラー文を漏らさない", func(t *testing.T) {
		store := &fakeStore{err: errors.New("connection refused: 10.0.1.23:5432")}
		rec := getNote(t, store, saved.ID.String())
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("状態コード：got %d, want 500", rec.Code)
		}
		assertProblem(t, rec, http.StatusInternalServerError)
		if strings.Contains(rec.Body.String(), "10.0.1.23") {
			t.Errorf("DB のエラー文が、応答に含まれている：%s", rec.Body.String())
		}
	})
}

// assertProblem は、応答が RFC 9457 の形のエラーであることを確かめる。
func assertProblem(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int) {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type：got %q, want application/problem+json", ct)
	}
	var p problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("本文を読めない：%v", err)
	}
	if p.Status != wantStatus || p.Title == "" || p.Detail == "" {
		t.Errorf("本文の形が正しくない：%+v", p)
	}
}
