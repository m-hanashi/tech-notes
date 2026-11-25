package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/m-hanashi/tech-notes/internal/summary"
)

// Summarizer は、メモの本文を AI で要約する部品に求めること。
// 本物は Bedrock を使う部品（#29 で作る）、テストでは偽物に差し替える。
type Summarizer interface {
	// Summarize は、本文の要約と、使ったモデルとトークン数を返す。
	// 回数制限で断られたときは、summary.ErrThrottled を包んだエラーを返す。
	Summarize(ctx context.Context, body string) (summary.Generated, error)
}

// SummaryStore は、要約の保存先に求めること。本物は postgres.SummaryStore、テストでは偽物に差し替える。
type SummaryStore interface {
	// Get は、メモの要約を返す。保存されていなければ summary.ErrNotFound を返す。
	Get(ctx context.Context, noteID uuid.UUID) (summary.Summary, error)
	// Save は、要約を保存する。すでに保存があれば、先のものを残し、エラーにしない。
	Save(ctx context.Context, s summary.Summary) error
}

// summaryTimeout は、要約1回で AI を待つ時間の上限（仕様書 5.2。#33 で実際の時間を測って見直す）
const summaryTimeout = 15 * time.Second

// saveTimeout は、作った要約の保存を待つ上限。保存は失敗してもよい（仕様書 4.4）ので、応答を長く遅らせない
const saveTimeout = 5 * time.Second

// retryAfterSeconds は、回数制限のときに、再試行まで待ってほしい秒数（仮の値。#29 で見直す）
const retryAfterSeconds = "10"

type summaryResponse struct {
	Summary string `json:"summary"`
}

type summaryHandler struct {
	store      NoteStore
	summaries  SummaryStore
	summarizer Summarizer
	timeout    time.Duration
}

// create は、POST /notes/{id}/summary を処理する。
// 保存した要約があれば、それを返す。なければ AI で作って保存し、返す（仕様書 5.2）。
func (h *summaryHandler) create(w http.ResponseWriter, r *http.Request) {
	n, ok := loadNote(w, r, h.store)
	if !ok {
		return
	}

	// 保存した要約があれば、AI を呼ばずに返す
	saved, err := h.summaries.Get(r.Context(), n.ID)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, summaryResponse{Summary: saved.Text})
		return
	case r.Context().Err() != nil:
		slog.Info("利用者が、要約を待たずに接続を切りました", "error", err)
		w.WriteHeader(statusClientClosedRequest)
		return
	case !errors.Is(err, summary.ErrNotFound):
		// 読み出せないときは、AI を呼ばない。DB の不調中に、料金のかかる呼び出しを増やさないため
		slog.Error("保存した要約を読み出せませんでした", "error", err)
		writeProblem(w, http.StatusInternalServerError, "要約の取得に失敗しました")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	start := time.Now()
	// AI に渡すのは本文だけ（仕様書 2.8。題名や id は渡さない）
	g, err := h.summarizer.Summarize(ctx, n.Body)
	latency := time.Since(start)

	if err != nil {
		writeSummaryError(ctx, w, r, h.timeout, err)
		return
	}
	if strings.TrimSpace(g.Text) == "" {
		slog.Error("AI が空の要約を返しました")
		writeProblemCode(w, http.StatusBadGateway, "要約を作れませんでした", "ai_error")
		return
	}

	// 保存に失敗しても、作った要約は返す。次の依頼で、もう一度作る（仕様書 4.4）。
	// 利用者がここで接続を切っても、料金を払って作った要約は保存する（WithoutCancel で、切断の合図を受け取らない）
	saveCtx, cancelSave := context.WithTimeout(context.WithoutCancel(r.Context()), saveTimeout)
	defer cancelSave()
	err = h.summaries.Save(saveCtx, summary.Summary{
		NoteID:       n.ID,
		Text:         g.Text,
		Model:        g.Model,
		InputTokens:  g.InputTokens,
		OutputTokens: g.OutputTokens,
		Latency:      latency,
		CreatedAt:    time.Now().UTC(),
	})
	if err != nil {
		slog.Error("要約を保存できませんでした", "error", err)
	}
	writeJSON(w, http.StatusOK, summaryResponse{Summary: g.Text})
}

// writeSummaryError は、AI の呼び出しの失敗を、種類ごとの応答にする（仕様書 5.2）。
// ctx は AI を待った上限つきの文脈、r.Context() は利用者の接続の文脈。
func writeSummaryError(ctx context.Context, w http.ResponseWriter, r *http.Request, timeout time.Duration, err error) {
	switch {
	case r.Context().Err() != nil:
		// 利用者が先に接続を切った。AI の失敗ではないので、5xx にしない
		slog.Info("利用者が、要約を待たずに接続を切りました", "error", err)
		w.WriteHeader(statusClientClosedRequest)
	// API 側の上限（ctx）か、AI の部品の中の上限のどちらかで、時間切れになった
	case errors.Is(ctx.Err(), context.DeadlineExceeded), errors.Is(err, context.DeadlineExceeded):
		slog.Warn("要約が時間内に終わりませんでした", "timeout", timeout)
		writeProblemCode(w, http.StatusGatewayTimeout, "要約が時間内に終わりませんでした", "ai_timeout")
	case errors.Is(err, summary.ErrThrottled):
		slog.Warn("AI の呼び出し回数の制限で、断られました", "error", err)
		w.Header().Set("Retry-After", retryAfterSeconds)
		writeProblemCode(w, http.StatusServiceUnavailable, "AI が混み合っています。しばらくしてから、もう一度試してください", "ai_throttled")
	default:
		// AI のエラー文は外に見せず、ログにだけ書く
		slog.Error("要約を作れませんでした", "error", err)
		writeProblemCode(w, http.StatusBadGateway, "要約を作れませんでした", "ai_error")
	}
}
