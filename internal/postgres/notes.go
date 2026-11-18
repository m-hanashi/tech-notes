// Package postgres は、PostgreSQL にデータを保存する部品をまとめる。
package postgres

import (
	"context"
	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL のドライバー（pgx）を "pgx" という名前で登録する

	"github.com/m-hanashi/tech-notes/internal/notes"
)

// Open は、DATABASE_URL の形の接続の指定で、PostgreSQL への接続を用意する。
// この時点では、まだ実際にはつながない（最初に使うときにつなぐ）。
func Open(dsn string) (*sql.DB, error) {
	return sql.Open("pgx", dsn)
}

// NoteStore は、notes テーブルにメモを保存する。httpapi.NoteStore の本物。
type NoteStore struct {
	db *sql.DB
}

// NewNoteStore は、db を使う NoteStore を返す。
func NewNoteStore(db *sql.DB) *NoteStore {
	return &NoteStore{db: db}
}

// Create は、メモを1件保存する。同じ id がすでにあれば、上書きせずにエラーを返す。
func (s *NoteStore) Create(ctx context.Context, n notes.Note) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO notes (id, title, body, created_at) VALUES ($1, $2, $3, $4)`,
		n.ID, n.Title, n.Body, n.CreatedAt,
	)
	return err
}
