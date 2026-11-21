// Package httpapi は、HTTP のリクエストを受け取り、応答を返す部分をまとめる。
package httpapi

import "net/http"

// NewHandler は、API 全体の入口を返す。認証してから、ルーターに渡す。main.go はこれを使う。
func NewHandler(store NoteStore, auth Authenticator) http.Handler {
	return requireAuth(auth, NewRouter(store))
}

// NewRouter は、メソッドとパスごとに、どのハンドラーで処理するかを決めたルーターを返す。
// 認証はしない（NewHandler で包む）。
// 登録していないパスには 404、登録したパスへの違うメソッドには 405 を、ルーターが自動で返す。
//
// store が nil のときは、メモの API を登録しない（DB につなぐまでの間、/health だけを動かすため）。
func NewRouter(store NoteStore) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)

	if store != nil {
		h := &notesHandler{store: store}
		mux.HandleFunc("POST /notes", h.create)
		mux.HandleFunc("GET /notes/{id}", h.get)
		mux.HandleFunc("GET /notes", h.list)
	}
	return mux
}
