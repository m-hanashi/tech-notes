package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/m-hanashi/tech-notes/internal/notes"
	"github.com/m-hanashi/tech-notes/internal/summary"
)

// fakeSummarizer は、テスト用の偽の AI。決めた要約かエラーを返し、渡された本文を覚える。
type fakeSummarizer struct {
	summary string
	err     error
	block   bool   // true なら、ctx が終わる（時間切れか、利用者が切る）まで返さない
	gotBody string // Summarize に渡された本文
	calls   int
}

func (s *fakeSummarizer) Summarize(ctx context.Context, body string) (string, error) {
	s.calls++
	s.gotBody = body
	if s.block {
		<-ctx.Done()
		return "", ctx.Err()
	}
	return s.summary, s.err
}

// テストでは、AI を待つ上限を短くする（15秒も待たないため）
const testSummaryTimeout = 10 * time.Millisecond

var savedNote = notes.Note{
	ID:        uuid.MustParse("0193a2f4-7c1e-7b3a-9f21-4c8d2e6b1a05"),
	Title:     "買い物",
	Body:      "牛乳を買う",
	CreatedAt: time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC),
}

func postSummary(store NoteStore, ai Summarizer, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	newRouter(store, ai, testSummaryTimeout).ServeHTTP(rec, req)
	return rec
}

func summaryRequest(id string) *http.Request {
	return httptest.NewRequest(http.MethodPost, "/notes/"+id+"/summary", nil)
}

// 1. 成功
func TestSummarize_Success(t *testing.T) {
	ai := &fakeSummarizer{summary: "- 牛乳を買う"}
	rec := postSummary(&fakeStore{created: []notes.Note{savedNote}}, ai, summaryRequest(savedNote.ID.String()))

	if rec.Code != http.StatusOK {
		t.Fatalf("状態コード：got %d, want 200（本文：%s）", rec.Code, rec.Body.String())
	}
	var got summaryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("本文を読めない：%v", err)
	}
	if got.Summary != "- 牛乳を買う" {
		t.Errorf("要約：got %q", got.Summary)
	}
	if ai.gotBody != savedNote.Body {
		t.Errorf("AI に渡した文章：got %q, want 本文だけ（%q）", ai.gotBody, savedNote.Body)
	}
}

// 2〜4. メモを取り出せないときは、AI を呼ばない
func TestSummarize_NoteErrors(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		store      *fakeStore
		wantStatus int
	}{
		{"2. ない id は 404", uuid.Must(uuid.NewV7()).String(), &fakeStore{}, http.StatusNotFound},
		{"3. UUID の形でない id は 400", "abc", &fakeStore{}, http.StatusBadRequest},
		{"4. DB の不調は 500", savedNote.ID.String(), &fakeStore{err: errors.New("connection refused")}, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ai := &fakeSummarizer{summary: "- 要約"}
			rec := postSummary(tt.store, ai, summaryRequest(tt.id))
			if rec.Code != tt.wantStatus {
				t.Fatalf("状態コード：got %d, want %d", rec.Code, tt.wantStatus)
			}
			assertProblem(t, rec, tt.wantStatus)
			if ai.calls != 0 {
				t.Errorf("AI が %d 回呼ばれた（料金がかかる）", ai.calls)
			}
		})
	}
}

// 5〜8. AI の失敗
func TestSummarize_AIErrors(t *testing.T) {
	tests := []struct {
		name       string
		ai         *fakeSummarizer
		wantStatus int
		wantCode   string
	}{
		{"5. 時間切れは 504", &fakeSummarizer{block: true}, http.StatusGatewayTimeout, "ai_timeout"},
		{"5. AI の部品の中の時間切れも 504", &fakeSummarizer{err: fmt.Errorf("bedrock: %w", context.DeadlineExceeded)}, http.StatusGatewayTimeout, "ai_timeout"},
		{"6. 回数制限は 503", &fakeSummarizer{err: fmt.Errorf("bedrock: %w", summary.ErrThrottled)}, http.StatusServiceUnavailable, "ai_throttled"},
		{"7. そのほかのエラーは 502", &fakeSummarizer{err: errors.New("ValidationException: 10.0.1.23")}, http.StatusBadGateway, "ai_error"},
		{"8. 空の要約は 502", &fakeSummarizer{summary: " \n "}, http.StatusBadGateway, "ai_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postSummary(&fakeStore{created: []notes.Note{savedNote}}, tt.ai, summaryRequest(savedNote.ID.String()))
			if rec.Code != tt.wantStatus {
				t.Fatalf("状態コード：got %d, want %d（本文：%s）", rec.Code, tt.wantStatus, rec.Body.String())
			}
			assertProblem(t, rec, tt.wantStatus)

			var p problem
			json.Unmarshal(rec.Body.Bytes(), &p)
			if p.Code != tt.wantCode {
				t.Errorf("code：got %q, want %q", p.Code, tt.wantCode)
			}
			if got := rec.Header().Get("Retry-After"); (tt.wantStatus == http.StatusServiceUnavailable) != (got != "") {
				t.Errorf("Retry-After：got %q（503 のときだけ付ける）", got)
			}
			if strings.Contains(rec.Body.String(), "10.0.1.23") {
				t.Errorf("AI のエラー文が、応答に含まれている：%s", rec.Body.String())
			}
		})
	}
}

// 9. AI が壊れていても、ほかの機能は動く
func TestSummarize_OtherAPIsWorkWhenAIFails(t *testing.T) {
	store := &fakeStore{created: []notes.Note{savedNote}}
	handler := newTestHandler(t, store, &fakeSummarizer{err: errors.New("AI が壊れている")})

	tests := []struct {
		method, path, body string
		wantStatus         int
	}{
		{http.MethodPost, "/notes/" + savedNote.ID.String() + "/summary", "", http.StatusBadGateway}, // AI が本当に壊れていることの確認
		{http.MethodPost, "/notes", `{"title":"題名","body":"本文"}`, http.StatusCreated},
		{http.MethodGet, "/notes/" + savedNote.ID.String(), "", http.StatusOK},
		{http.MethodGet, "/notes", "", http.StatusOK},
		{http.MethodGet, "/health", "", http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("X-API-Key", testAPIKey)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("状態コード：got %d, want %d（本文：%s）", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

// 10.（API キーなしは 401）は、ほかの API とまとめて auth_test.go の TestAuth で確かめる

// 11. 利用者が途中で接続を切ったときは、AI の失敗（5xx）として数えない
func TestSummarize_ClientGone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 利用者が接続を切った状態にする
	req := summaryRequest(savedNote.ID.String()).WithContext(ctx)

	rec := postSummary(&fakeStore{created: []notes.Note{savedNote}}, &fakeSummarizer{block: true}, req)
	if rec.Code != statusClientClosedRequest {
		t.Errorf("状態コード：got %d, want %d", rec.Code, statusClientClosedRequest)
	}
}
