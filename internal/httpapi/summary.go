package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/m-hanashi/tech-notes/internal/summary"
)

// Summarizer は、メモの本文を AI で要約する部品に求めること。
// 本物は Bedrock を使う部品（#29 で作る）、テストでは偽物に差し替える。
type Summarizer interface {
	// Summarize は、本文の要約を返す。
	// 回数制限で断られたときは、summary.ErrThrottled を包んだエラーを返す。
	Summarize(ctx context.Context, body string) (string, error)
}

// summaryTimeout は、要約1回で AI を待つ時間の上限（仕様書 5.2。#33 で実際の時間を測って見直す）
const summaryTimeout = 15 * time.Second

// retryAfterSeconds は、回数制限のときに、再試行まで待ってほしい秒数（仮の値。#29 で見直す）
const retryAfterSeconds = "10"

type summaryResponse struct {
	Summary string `json:"summary"`
}

type summaryHandler struct {
	store      NoteStore
	summarizer Summarizer
	timeout    time.Duration
}

// create は、POST /notes/{id}/summary を処理する。
func (h *summaryHandler) create(w http.ResponseWriter, r *http.Request) {
	n, ok := loadNote(w, r, h.store)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	// AI に渡すのは本文だけ（仕様書 2.8。題名や id は渡さない）
	s, err := h.summarizer.Summarize(ctx, n.Body)

	if err == nil {
		if strings.TrimSpace(s) == "" {
			slog.Error("AI が空の要約を返しました")
			writeProblemCode(w, http.StatusBadGateway, "要約を作れませんでした", "ai_error")
			return
		}
		writeJSON(w, http.StatusOK, summaryResponse{Summary: s})
		return
	}

	switch {
	case r.Context().Err() != nil:
		// 利用者が先に接続を切った。AI の失敗ではないので、5xx にしない
		slog.Info("利用者が、要約を待たずに接続を切りました", "error", err)
		w.WriteHeader(statusClientClosedRequest)
	// API 側の上限（ctx）か、AI の部品の中の上限のどちらかで、時間切れになった
	case errors.Is(ctx.Err(), context.DeadlineExceeded), errors.Is(err, context.DeadlineExceeded):
		slog.Warn("要約が時間内に終わりませんでした", "timeout", h.timeout)
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
