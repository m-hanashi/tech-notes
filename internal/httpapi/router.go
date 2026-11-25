// Package httpapi は、HTTP のリクエストを受け取り、応答を返す部分をまとめる。
package httpapi

import (
	"net/http"
	"time"
)

// NewHandler は、API 全体の入口を返す。認証してから、ルーターに渡す。main.go はこれを使う。
func NewHandler(store NoteStore, summaries SummaryStore, summarizer Summarizer, auth Authenticator) http.Handler {
	return requireAuth(auth, NewRouter(store, summaries, summarizer))
}

// NewRouter は、メソッドとパスごとに、どのハンドラーで処理するかを決めたルーターを返す。
// 認証はしない（NewHandler で包む）。
// 登録していないパスには 404、登録したパスへの違うメソッドには 405 を、ルーターが自動で返す。
//
// store が nil のときは、メモの API を登録しない（DB につなぐまでの間、/health だけを動かすため）。
// summaries か summarizer が nil のときは、要約の API を登録しない（AI の部品ができるまでの間）。
func NewRouter(store NoteStore, summaries SummaryStore, summarizer Summarizer) http.Handler {
	return newRouter(store, summaries, summarizer, summaryTimeout)
}

// newRouter は、AI を待つ上限の時間を指定できる。テストで、15秒待たずに時間切れを起こすため。
func newRouter(store NoteStore, summaries SummaryStore, summarizer Summarizer, timeout time.Duration) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)

	if store != nil {
		h := &notesHandler{store: store}
		mux.HandleFunc("POST /notes", h.create)
		mux.HandleFunc("GET /notes/{id}", h.get)
		mux.HandleFunc("GET /notes", h.list)
	}
	if store != nil && summaries != nil && summarizer != nil {
		s := &summaryHandler{store: store, summaries: summaries, summarizer: summarizer, timeout: timeout}
		mux.HandleFunc("POST /notes/{id}/summary", s.create)
	}
	return mux
}
