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
	created  []notes.Note
	err      error
	gotLimit int // List に渡された limit
	calls    int // Create・Get・List のどれかが呼ばれた回数
}

func (s *fakeStore) Create(_ context.Context, n notes.Note) error {
	s.calls++
	if s.err != nil {
		return s.err
	}
	s.created = append(s.created, n)
	return nil
}

func (s *fakeStore) Get(_ context.Context, id uuid.UUID) (notes.Note, error) {
	s.calls++
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

// List は、受け取った limit を覚えておき、保存したメモを新しい順（入れた順の逆）に返す。
func (s *fakeStore) List(_ context.Context, limit int) ([]notes.Note, error) {
	s.calls++
	s.gotLimit = limit
	if s.err != nil {
		return nil, s.err
	}
	var list []notes.Note
	for i := len(s.created) - 1; i >= 0 && len(list) < limit; i-- {
		list = append(list, s.created[i])
	}
	return list, nil
}

func postNote(t *testing.T, store NoteStore, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/notes", strings.NewReader(body))
	rec := httptest.NewRecorder()
	NewRouter(store, nil).ServeHTTP(rec, req)
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
		{"JSON の後ろに余計なデータがある", `{"title":"題名","body":"本文"}{"x":1}`, http.StatusBadRequest},
		{"title に NUL 文字がある", `{"title":"a\u0000b","body":"本文"}`, http.StatusBadRequest},
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
	NewRouter(store, nil).ServeHTTP(rec, req)
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

	t.Run("DB を待つ間に利用者が切ったら、DB の失敗（500）でなく 499 を返す", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // 利用者が接続を切った状態にする
		req := httptest.NewRequest(http.MethodGet, "/notes/"+saved.ID.String(), nil).WithContext(ctx)
		rec := httptest.NewRecorder()
		NewRouter(&fakeStore{err: context.Canceled}, nil).ServeHTTP(rec, req) // DB が「打ち切られた」と返す
		if rec.Code != statusClientClosedRequest {
			t.Errorf("状態コード：got %d, want %d", rec.Code, statusClientClosedRequest)
		}
	})
}

func listNotes(t *testing.T, store NoteStore, query string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/notes"+query, nil)
	rec := httptest.NewRecorder()
	NewRouter(store, nil).ServeHTTP(rec, req)
	return rec
}

func TestListNotes(t *testing.T) {
	t.Run("新しい順に並ぶ", func(t *testing.T) {
		older, newer := notes.Note{Title: "古い"}, notes.Note{Title: "新しい"}
		rec := listNotes(t, &fakeStore{created: []notes.Note{older, newer}}, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("状態コード：got %d, want 200", rec.Code)
		}
		var got listNotesResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("本文を読めない：%v", err)
		}
		if len(got.Notes) != 2 || got.Notes[0].Title != "新しい" || got.Notes[1].Title != "古い" {
			t.Errorf("並び順：got %+v", got.Notes)
		}
	})

	t.Run("1件もないときは [] を返す", func(t *testing.T) {
		rec := listNotes(t, &fakeStore{}, "")
		if got := strings.TrimSpace(rec.Body.String()); got != `{"notes":[]}` {
			t.Errorf("本文：got %s, want {\"notes\":[]}", got)
		}
	})

	// limit の扱い。保存先に渡された limit で確かめる（件数の打ち切りは、DB のテストで確かめる）
	tests := []struct {
		name       string
		query      string
		wantStatus int
		wantLimit  int // 200 のときに、保存先に渡されるべき limit
	}{
		{"limit なしは 20", "", http.StatusOK, 20},
		{"limit=5 は 5", "?limit=5", http.StatusOK, 5},
		{"limit=100 は 100", "?limit=100", http.StatusOK, 100},
		{"limit=101 は 400", "?limit=101", http.StatusBadRequest, 0},
		{"limit=0 は 400", "?limit=0", http.StatusBadRequest, 0},
		{"limit=-1 は 400", "?limit=-1", http.StatusBadRequest, 0},
		{"limit=abc は 400", "?limit=abc", http.StatusBadRequest, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeStore{}
			rec := listNotes(t, store, tt.query)
			if rec.Code != tt.wantStatus {
				t.Fatalf("状態コード：got %d, want %d（本文：%s）", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantStatus == http.StatusOK && store.gotLimit != tt.wantLimit {
				t.Errorf("保存先に渡した limit：got %d, want %d", store.gotLimit, tt.wantLimit)
			}
			if tt.wantStatus >= 400 {
				assertProblem(t, rec, tt.wantStatus)
			}
		})
	}
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
