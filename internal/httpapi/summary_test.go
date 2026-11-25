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
	onDone  func() // 要約を返す直前に呼ぶ（返した直後に利用者が接続を切る場面を作る）
	gotBody string // Summarize に渡された本文
	calls   int
}

func (s *fakeSummarizer) Summarize(ctx context.Context, body string) (summary.Generated, error) {
	s.calls++
	s.gotBody = body
	if s.block {
		<-ctx.Done()
		return summary.Generated{}, ctx.Err()
	}
	if s.onDone != nil {
		s.onDone()
	}
	return summary.Generated{Text: s.summary, Model: "fake-model", InputTokens: 100, OutputTokens: 20}, s.err
}

// fakeSummaryStore は、テスト用の偽の要約の保存先。本物と同じく、すでに保存があれば先のものを残す。
type fakeSummaryStore struct {
	stored   *summary.Summary // 保存されている要約。nil なら、まだない
	staleGet bool             // true なら、Get は保存があっても「ない」と答える（同時の依頼で、相手がまだ保存していなかった瞬間を作る）
	getErr   error
	saveErr  error
	saves    int // Save が呼ばれた回数
}

func (s *fakeSummaryStore) Get(ctx context.Context, noteID uuid.UUID) (summary.Summary, error) {
	if s.getErr != nil {
		return summary.Summary{}, s.getErr
	}
	if s.stored == nil || s.staleGet || s.stored.NoteID != noteID {
		return summary.Summary{}, summary.ErrNotFound
	}
	return *s.stored, nil
}

func (s *fakeSummaryStore) Save(ctx context.Context, sm summary.Summary) error {
	s.saves++
	if s.saveErr != nil {
		return s.saveErr
	}
	if err := ctx.Err(); err != nil { // 本物の DB と同じく、取り消された文脈では保存しない
		return err
	}
	if s.stored == nil { // 先勝ち
		s.stored = &sm
	}
	return nil
}

// テストでは、AI を待つ上限を短くする（15秒も待たないため）
const testSummaryTimeout = 10 * time.Millisecond

var savedNote = notes.Note{
	ID:        uuid.MustParse("0193a2f4-7c1e-7b3a-9f21-4c8d2e6b1a05"),
	Title:     "買い物",
	Body:      "牛乳を買う",
	CreatedAt: time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC),
}

// storedSummary は、savedNote の要約がすでに保存されている状態を作る。
func storedSummary(text string) *summary.Summary {
	return &summary.Summary{NoteID: savedNote.ID, Text: text, Model: "fake-model"}
}

func postSummary(store NoteStore, summaries SummaryStore, ai Summarizer, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	newRouter(store, summaries, ai, testSummaryTimeout).ServeHTTP(rec, req)
	return rec
}

func summaryRequest(id string) *http.Request {
	return httptest.NewRequest(http.MethodPost, "/notes/"+id+"/summary", nil)
}

// summaryText は、200 の応答の本文から、要約の文章を取り出す。
func summaryText(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("状態コード：got %d, want 200（本文：%s）", rec.Code, rec.Body.String())
	}
	var got summaryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("本文を読めない：%v", err)
	}
	return got.Summary
}

// 1. 成功（保存がない1回目）。AI を呼び、作った要約を保存する
func TestSummarize_Success(t *testing.T) {
	ai := &fakeSummarizer{summary: "- 牛乳を買う"}
	summaries := &fakeSummaryStore{}
	rec := postSummary(&fakeStore{created: []notes.Note{savedNote}}, summaries, ai, summaryRequest(savedNote.ID.String()))

	if got := summaryText(t, rec); got != "- 牛乳を買う" {
		t.Errorf("要約：got %q", got)
	}
	if ai.gotBody != savedNote.Body {
		t.Errorf("AI に渡した文章：got %q, want 本文だけ（%q）", ai.gotBody, savedNote.Body)
	}
	s := summaries.stored
	if s == nil {
		t.Fatal("要約が保存されていない")
	}
	if s.NoteID != savedNote.ID || s.Text != "- 牛乳を買う" || s.Model != "fake-model" || s.InputTokens != 100 || s.OutputTokens != 20 || s.CreatedAt.IsZero() {
		t.Errorf("保存した内容：got %+v", *s)
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
			rec := postSummary(tt.store, &fakeSummaryStore{}, ai, summaryRequest(tt.id))
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

// 5〜8. AI の失敗。失敗は保存しない
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
			summaries := &fakeSummaryStore{}
			rec := postSummary(&fakeStore{created: []notes.Note{savedNote}}, summaries, tt.ai, summaryRequest(savedNote.ID.String()))
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
			if summaries.saves != 0 {
				t.Errorf("失敗なのに、保存が %d 回呼ばれた（失敗が返り続けてしまう）", summaries.saves)
			}
		})
	}
}

// 9. AI が壊れていても、ほかの機能は動く
func TestSummarize_OtherAPIsWorkWhenAIFails(t *testing.T) {
	store := &fakeStore{created: []notes.Note{savedNote}}
	handler := newTestHandler(t, store, &fakeSummaryStore{}, &fakeSummarizer{err: errors.New("AI が壊れている")})

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

	rec := postSummary(&fakeStore{created: []notes.Note{savedNote}}, &fakeSummaryStore{}, &fakeSummarizer{block: true}, req)
	if rec.Code != statusClientClosedRequest {
		t.Errorf("状態コード：got %d, want %d", rec.Code, statusClientClosedRequest)
	}
}

// 12〜13. 保存した要約があれば、AI を呼ばずに返す（AI が壊れていても）
func TestSummarize_ReturnsStored(t *testing.T) {
	tests := []struct {
		name string
		ai   *fakeSummarizer
	}{
		{"12. 2回目は、保存した要約を返す", &fakeSummarizer{summary: "- AI が新しく作った要約"}},
		{"13. AI が壊れていても、保存した要約を返す", &fakeSummarizer{err: errors.New("AI が壊れている")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summaries := &fakeSummaryStore{stored: storedSummary("- 保存した要約")}
			rec := postSummary(&fakeStore{created: []notes.Note{savedNote}}, summaries, tt.ai, summaryRequest(savedNote.ID.String()))

			if got := summaryText(t, rec); got != "- 保存した要約" {
				t.Errorf("要約：got %q, want 保存した要約", got)
			}
			if tt.ai.calls != 0 {
				t.Errorf("AI が %d 回呼ばれた（料金がかかる）", tt.ai.calls)
			}
		})
	}
}

// 14. 同時の依頼の2つ目：保存しようとしたら、もう相手の要約があった。エラーにせず、自分が作った要約を返す
func TestSummarize_ConcurrentSecondReturnsOwn(t *testing.T) {
	summaries := &fakeSummaryStore{stored: storedSummary("- 相手が先に保存した要約"), staleGet: true}
	ai := &fakeSummarizer{summary: "- 自分が作った要約"}
	rec := postSummary(&fakeStore{created: []notes.Note{savedNote}}, summaries, ai, summaryRequest(savedNote.ID.String()))

	if got := summaryText(t, rec); got != "- 自分が作った要約" {
		t.Errorf("要約：got %q, want 自分が作った要約", got)
	}
	if summaries.stored.Text != "- 相手が先に保存した要約" {
		t.Errorf("保存：got %q, want 先に保存した要約のまま", summaries.stored.Text)
	}
	if ai.calls != 1 || summaries.saves != 1 {
		t.Errorf("AI が %d 回、保存が %d 回呼ばれた（どちらも1回のはず）", ai.calls, summaries.saves)
	}
}

// 15. 保存に失敗しても、作った要約は返す
func TestSummarize_SaveFailsStillReturns(t *testing.T) {
	summaries := &fakeSummaryStore{saveErr: errors.New("connection refused")}
	ai := &fakeSummarizer{summary: "- 牛乳を買う"}
	rec := postSummary(&fakeStore{created: []notes.Note{savedNote}}, summaries, ai, summaryRequest(savedNote.ID.String()))

	if got := summaryText(t, rec); got != "- 牛乳を買う" {
		t.Errorf("要約：got %q", got)
	}
	if ai.calls != 1 || summaries.saves != 1 {
		t.Errorf("AI が %d 回、保存が %d 回呼ばれた（どちらも1回のはず）", ai.calls, summaries.saves)
	}
}

// 16. 保存した要約を読み出せないときは 500。AI は呼ばない
func TestSummarize_GetFailsReturns500(t *testing.T) {
	summaries := &fakeSummaryStore{getErr: errors.New("connection refused")}
	ai := &fakeSummarizer{summary: "- 要約"}
	rec := postSummary(&fakeStore{created: []notes.Note{savedNote}}, summaries, ai, summaryRequest(savedNote.ID.String()))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("状態コード：got %d, want 500", rec.Code)
	}
	assertProblem(t, rec, http.StatusInternalServerError)
	if ai.calls != 0 {
		t.Errorf("AI が %d 回呼ばれた（料金がかかる）", ai.calls)
	}
}

// 17. 保存した要約を探している最中に利用者が接続を切ったら 499。AI は呼ばない
func TestSummarize_ClientGoneWhileGetting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 利用者が接続を切った状態にする
	req := summaryRequest(savedNote.ID.String()).WithContext(ctx)
	summaries := &fakeSummaryStore{getErr: context.Canceled} // DB が「打ち切られた」と返す
	ai := &fakeSummarizer{summary: "- 要約"}

	rec := postSummary(&fakeStore{created: []notes.Note{savedNote}}, summaries, ai, req)
	if rec.Code != statusClientClosedRequest {
		t.Errorf("状態コード：got %d, want %d", rec.Code, statusClientClosedRequest)
	}
	if ai.calls != 0 {
		t.Errorf("AI が %d 回呼ばれた（料金がかかる）", ai.calls)
	}
}

// 18. AI が要約を返した直後に利用者が接続を切っても、要約は保存する（料金を払った要約を捨てない）
func TestSummarize_ClientGoneAfterAIStillSaves(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := summaryRequest(savedNote.ID.String()).WithContext(ctx)
	summaries := &fakeSummaryStore{}
	ai := &fakeSummarizer{summary: "- 牛乳を買う", onDone: cancel} // 要約を返す直前に、接続が切れる

	postSummary(&fakeStore{created: []notes.Note{savedNote}}, summaries, ai, req)
	if summaries.stored == nil || summaries.stored.Text != "- 牛乳を買う" {
		t.Errorf("接続が切れたあと、要約が保存されていない：got %+v", summaries.stored)
	}
}
