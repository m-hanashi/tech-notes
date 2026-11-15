package httpapi

import "net/http"

// handleHealth は、API が動いていることを返す。
// DB や AI の状態は見ない。Aurora が自動休止から目覚めるまでの間に、
// 「壊れている」と判定されないようにするため（仕様書 4.4、5.2）。
func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}
