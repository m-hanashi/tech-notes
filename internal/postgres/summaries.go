package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/m-hanashi/tech-notes/internal/summary"
)

// SummaryStore は、summaries テーブルに AI の要約を保存する。
type SummaryStore struct {
	db *sql.DB
}

// NewSummaryStore は、db を使う SummaryStore を返す。
func NewSummaryStore(db *sql.DB) *SummaryStore {
	return &SummaryStore{db: db}
}

// Save は、要約を1件保存する。
// そのメモの要約がすでに保存されていれば、先のものを残し、エラーにもしない（仕様書 4.4。同時の依頼に備える）。
// CreatedAt が空（ゼロ値）なら、DB の現在時刻を使う。入れ忘れで 0001-01-01 が保存されるのを防ぐため。
func (s *SummaryStore) Save(ctx context.Context, sm summary.Summary) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	var createdAt any // nil のままなら、SQL の COALESCE で now() になる
	if !sm.CreatedAt.IsZero() {
		createdAt = sm.CreatedAt
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO summaries (note_id, summary, model, input_tokens, output_tokens, latency_ms, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, COALESCE($7, now()))
		 ON CONFLICT (note_id) DO NOTHING`,
		sm.NoteID, sm.Text, sm.Model, sm.InputTokens, sm.OutputTokens, sm.Latency.Milliseconds(), createdAt,
	)
	return err
}

// Get は、メモの id に対応する要約を返す。なければ summary.ErrNotFound を返す。
func (s *SummaryStore) Get(ctx context.Context, noteID uuid.UUID) (summary.Summary, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	var sm summary.Summary
	var latencyMS int64
	err := s.db.QueryRowContext(ctx,
		`SELECT note_id, summary, model, input_tokens, output_tokens, latency_ms, created_at
		 FROM summaries WHERE note_id = $1`, noteID,
	).Scan(&sm.NoteID, &sm.Text, &sm.Model, &sm.InputTokens, &sm.OutputTokens, &latencyMS, &sm.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return summary.Summary{}, summary.ErrNotFound
	}
	if err != nil {
		return summary.Summary{}, err
	}
	sm.Latency = time.Duration(latencyMS) * time.Millisecond
	sm.CreatedAt = sm.CreatedAt.UTC()
	return sm, nil
}
