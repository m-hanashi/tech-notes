package postgres

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"

	"github.com/m-hanashi/tech-notes/internal/notes"
	"github.com/m-hanashi/tech-notes/migrations"
)

// openTestDB は、DATABASE_URL の PostgreSQL につなぎ、マイグレーションを適用して、notes を空にする。
// DATABASE_URL がなければ、テストを飛ばす（手元で DB を起動していないとき）。
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL がないので、DB を使うテストを飛ばす")
	}
	db, err := Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err := goose.Up(db, "."); err != nil {
		t.Fatalf("マイグレーションに失敗した：%v", err)
	}
	if _, err := db.Exec(`TRUNCATE notes`); err != nil {
		t.Fatal(err)
	}
	return db
}

func newNote(title string) notes.Note {
	return notes.Note{
		ID:        uuid.Must(uuid.NewV7()),
		Title:     title,
		Body:      "本文",
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
}

func TestCreate_SavesNote(t *testing.T) {
	db := openTestDB(t)
	store := NewNoteStore(db)
	want := newNote("日本語の題名")

	if err := store.Create(context.Background(), want); err != nil {
		t.Fatalf("保存に失敗した：%v", err)
	}

	var got notes.Note
	err := db.QueryRow(`SELECT id, title, body, created_at FROM notes WHERE id = $1`, want.ID).
		Scan(&got.ID, &got.Title, &got.Body, &got.CreatedAt)
	if err != nil {
		t.Fatalf("読み出しに失敗した：%v", err)
	}
	if got.ID != want.ID || got.Title != want.Title || got.Body != want.Body || !got.CreatedAt.Equal(want.CreatedAt) {
		t.Errorf("保存した内容が違う：got %+v, want %+v", got, want)
	}
}

func TestCreate_RejectsDuplicateID(t *testing.T) {
	store := NewNoteStore(openTestDB(t))
	n := newNote("題名")

	if err := store.Create(context.Background(), n); err != nil {
		t.Fatalf("1回目の保存に失敗した：%v", err)
	}
	if err := store.Create(context.Background(), n); err == nil {
		t.Error("同じ id の2回目の保存が、エラーにならなかった")
	}
}

func TestCreate_RejectsTooLongTitle(t *testing.T) {
	store := NewNoteStore(openTestDB(t))
	n := newNote(strings.Repeat("あ", notes.MaxTitleLength+1))

	if err := store.Create(context.Background(), n); err == nil {
		t.Error("201文字の title が、DB の CHECK 制約で拒否されなかった")
	}
}

func TestCreate_ErrorWhenDBUnreachable(t *testing.T) {
	// 誰も待っていないポートにつなぐ。本物の DB は使わないので、飛ばさない。
	db, err := Open("postgres://technotes:technotes@127.0.0.1:1/technotes?sslmode=disable&connect_timeout=2")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := NewNoteStore(db).Create(context.Background(), newNote("題名")); err == nil {
		t.Error("DB につながらないのに、エラーにならなかった")
	}
}
