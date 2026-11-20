package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/m-hanashi/tech-notes/internal/notes"
)

// maxRequestBytes は、受け取る本文の大きさの上限。これを超える本文は、読む前に 413 で断る。
const maxRequestBytes = 1 << 20 // 1MB

// NoteStore は、メモを保存し、読み出す部品に求めること。
// 本物は PostgreSQL を使う部品（internal/postgres）、テストでは偽物に差し替える。
type NoteStore interface {
	Create(ctx context.Context, n notes.Note) error
	// Get は、id のメモを返す。なければ notes.ErrNotFound を返す。
	Get(ctx context.Context, id uuid.UUID) (notes.Note, error)
}

type notesHandler struct {
	store NoteStore
}

type createNoteRequest struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// create は、POST /notes を処理する。
func (h *notesHandler) create(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields() // 知らない項目（書き間違いなど）があれば断る

	var req createNoteRequest
	if err := dec.Decode(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeProblem(w, http.StatusRequestEntityTooLarge, "本文は1MB以内にしてください")
			return
		}
		writeProblem(w, http.StatusBadRequest, "本文の JSON が正しくありません")
		return
	}

	if err := notes.Validate(req.Title, req.Body); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}

	id, err := uuid.NewV7()
	if err != nil {
		slog.Error("id を作れませんでした", "error", err)
		writeProblem(w, http.StatusInternalServerError, "メモの登録に失敗しました")
		return
	}
	n := notes.Note{
		ID:    id,
		Title: req.Title,
		Body:  req.Body,
		// PostgreSQL の日時はマイクロ秒までなので、返す値と保存する値をそろえる
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
	}

	if err := h.store.Create(r.Context(), n); err != nil {
		// DB のエラー文は外に見せず、ログにだけ書く
		slog.Error("メモを保存できませんでした", "error", err)
		writeProblem(w, http.StatusInternalServerError, "メモの登録に失敗しました")
		return
	}

	w.Header().Set("Location", "/notes/"+id.String())
	writeJSON(w, http.StatusCreated, n)
}

// get は、GET /notes/{id} を処理する。
func (h *notesHandler) get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "id は UUID の形で指定してください")
		return
	}

	n, err := h.store.Get(r.Context(), id)
	if errors.Is(err, notes.ErrNotFound) {
		writeProblem(w, http.StatusNotFound, "メモが見つかりません")
		return
	}
	if err != nil {
		// DB のエラー文は外に見せず、ログにだけ書く
		slog.Error("メモを読み出せませんでした", "error", err)
		writeProblem(w, http.StatusInternalServerError, "メモの取得に失敗しました")
		return
	}

	writeJSON(w, http.StatusOK, n)
}
