// Package httpapi は、HTTP のリクエストを受け取り、応答を返す部分をまとめる。
package httpapi

import "net/http"

// NewRouter は、メソッドとパスごとに、どのハンドラーで処理するかを決めたルーターを返す。
// 登録していないパスには 404、登録したパスへの違うメソッドには 405 を、ルーターが自動で返す。
func NewRouter() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)
	return mux
}
