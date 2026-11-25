package postgres

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"

	"github.com/m-hanashi/tech-notes/internal/notes"
	"github.com/m-hanashi/tech-notes/migrations"
)

// openTestDB は、DATABASE_URL の PostgreSQL につなぎ、マイグレーションを適用して、テーブルを空にする。
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
	// summaries は notes を参照しているので、一緒に空にする（notes だけでは PostgreSQL が断る）
	if _, err := db.Exec(`TRUNCATE summaries, notes`); err != nil {
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

func TestGet_ReturnsSavedNote(t *testing.T) {
	store := NewNoteStore(openTestDB(t))
	want := newNote("日本語の題名")
	if err := store.Create(context.Background(), want); err != nil {
		t.Fatalf("保存に失敗した：%v", err)
	}

	got, err := store.Get(context.Background(), want.ID)
	if err != nil {
		t.Fatalf("読み出しに失敗した：%v", err)
	}
	if got != want {
		t.Errorf("読み出した内容が違う：got %+v, want %+v", got, want)
	}
}

func TestList_NewestFirstAndLimit(t *testing.T) {
	store := NewNoteStore(openTestDB(t))
	base := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	for i, title := range []string{"1番目", "2番目", "3番目"} {
		n := newNote(title)
		n.CreatedAt = base.Add(time.Duration(i) * time.Minute) // 1分ずつ新しくする
		if err := store.Create(context.Background(), n); err != nil {
			t.Fatalf("保存に失敗した：%v", err)
		}
	}

	got, err := store.List(context.Background(), 2)
	if err != nil {
		t.Fatalf("一覧の読み出しに失敗した：%v", err)
	}
	if len(got) != 2 || got[0].Title != "3番目" || got[1].Title != "2番目" {
		t.Errorf("新しい順に2件：got %+v", got)
	}
}

func TestGet_NotFound(t *testing.T) {
	store := NewNoteStore(openTestDB(t))

	_, err := store.Get(context.Background(), uuid.Must(uuid.NewV7()))
	if !errors.Is(err, notes.ErrNotFound) {
		t.Errorf("ない id のエラー：got %v, want notes.ErrNotFound", err)
	}
}
