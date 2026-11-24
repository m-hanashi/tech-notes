package httpapi

import (
	"encoding/json"
	"net/http"
)

// statusClientClosedRequest は、利用者が応答を待たずに接続を切ったことを表す（nginx の 499 にならう）。
// HTTP の標準にはない番号。DB や AI の失敗（5xx）と分けて数えるために使う。
const statusClientClosedRequest = 499

// problem は、エラーを返すときの本文の形（RFC 9457 の Problem Details）。
type problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
	// Code は、機械が読むための失敗の理由（例：ai_timeout）。RFC 9457 で認められている拡張の項目
	Code string `json:"code,omitempty"`
}

// writeProblem は、状態コードと理由を、RFC 9457 の形の JSON で返す。
func writeProblem(w http.ResponseWriter, status int, detail string) {
	writeProblemCode(w, status, detail, "")
}

// writeProblemCode は、writeProblem に、機械が読むための失敗の理由（code）を足して返す。
func writeProblemCode(w http.ResponseWriter, status int, detail, code string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(problem{
		Type:   "about:blank",
		Title:  http.StatusText(status),
		Status: status,
		Detail: detail,
		Code:   code,
	})
}

// writeJSON は、成功したときの本文を JSON で返す。
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
