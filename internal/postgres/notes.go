// Package postgres は、PostgreSQL にデータを保存する部品をまとめる。
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL のドライバー（pgx）を "pgx" という名前で登録する

	"github.com/m-hanashi/tech-notes/internal/notes"
)

// queryTimeout は、DB への1回の問い合わせを待つ上限。
// Aurora が自動休止から目覚めるまでの時間（十数秒）を見込んだ値。段階3で、実際の時間を見て調整する。
const queryTimeout = 30 * time.Second

// Open は、DATABASE_URL の形の接続の指定で、PostgreSQL への接続を用意する。
// この時点では、まだ実際にはつながない（最初に使うときにつなぐ）。
func Open(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	// 使っていない接続は1分で閉じる。Aurora Serverless v2 は、接続が0本にならないと自動休止しないため
	db.SetConnMaxIdleTime(time.Minute)
	return db, nil
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
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO notes (id, title, body, created_at) VALUES ($1, $2, $3, $4)`,
		n.ID, n.Title, n.Body, n.CreatedAt,
	)
	return err
}

// Get は、id のメモを返す。なければ notes.ErrNotFound を返す。
func (s *NoteStore) Get(ctx context.Context, id uuid.UUID) (notes.Note, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	var n notes.Note
	err := s.db.QueryRowContext(ctx,
		`SELECT id, title, body, created_at FROM notes WHERE id = $1`, id,
	).Scan(&n.ID, &n.Title, &n.Body, &n.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return notes.Note{}, notes.ErrNotFound
	}
	if err != nil {
		return notes.Note{}, err
	}
	n.CreatedAt = n.CreatedAt.UTC()
	return n, nil
}

// List は、新しい順に、最大 limit 件のメモを返す。
// 同じ時刻のメモは、id の大きい順に並べる（UUID v7 は作った順に大きくなる）。
func (s *NoteStore) List(ctx context.Context, limit int) ([]notes.Note, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, title, body, created_at FROM notes ORDER BY created_at DESC, id DESC LIMIT $1`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []notes.Note
	for rows.Next() {
		var n notes.Note
		if err := rows.Scan(&n.ID, &n.Title, &n.Body, &n.CreatedAt); err != nil {
			return nil, err
		}
		n.CreatedAt = n.CreatedAt.UTC()
		list = append(list, n)
	}
	return list, rows.Err()
}
