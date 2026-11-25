package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/m-hanashi/tech-notes/internal/summary"
)

// newSavedNoteAndSummary は、メモを1件保存し、そのメモの要約（まだ保存しない）を返す。
// 要約はメモを参照するので、先にメモを保存しておく必要がある。
func newSavedNoteAndSummary(t *testing.T, store *NoteStore, text string) summary.Summary {
	t.Helper()
	n := newNote("題名")
	if err := store.Create(context.Background(), n); err != nil {
		t.Fatalf("メモの保存に失敗した：%v", err)
	}
	return summary.Summary{
		NoteID:       n.ID,
		Text:         text,
		Model:        "test-model",
		InputTokens:  120,
		OutputTokens: 45,
		Latency:      1234 * time.Millisecond,
		CreatedAt:    time.Now().UTC().Truncate(time.Microsecond),
	}
}

// 登録して引くと、同じ値が返る
func TestSummary_SaveThenGet(t *testing.T) {
	db := openTestDB(t)
	summaries := NewSummaryStore(db)
	want := newSavedNoteAndSummary(t, NewNoteStore(db), "- 牛乳を買う")

	if err := summaries.Save(context.Background(), want); err != nil {
		t.Fatalf("保存に失敗した：%v", err)
	}
	got, err := summaries.Get(context.Background(), want.NoteID)
	if err != nil {
		t.Fatalf("読み出しに失敗した：%v", err)
	}
	// 時刻は == ではなく Equal で比べる（同じ時刻でも、内部の持ち方が違うことがあるため）
	if !got.CreatedAt.Equal(want.CreatedAt) {
		t.Errorf("作成時刻：got %v, want %v", got.CreatedAt, want.CreatedAt)
	}
	got.CreatedAt = want.CreatedAt // 時刻は確かめたので、残りの項目をまとめて比べる
	if got != want {
		t.Errorf("読み出した内容が違う：got %+v, want %+v", got, want)
	}
}

// 作成時刻を入れ忘れても、0001-01-01 ではなく、保存した時刻になる
func TestSummary_SaveWithoutCreatedAtUsesNow(t *testing.T) {
	db := openTestDB(t)
	summaries := NewSummaryStore(db)
	sm := newSavedNoteAndSummary(t, NewNoteStore(db), "- 牛乳を買う")
	sm.CreatedAt = time.Time{} // 入れ忘れた状態
	before := time.Now().Add(-time.Minute)

	if err := summaries.Save(context.Background(), sm); err != nil {
		t.Fatalf("保存に失敗した：%v", err)
	}
	got, err := summaries.Get(context.Background(), sm.NoteID)
	if err != nil {
		t.Fatalf("読み出しに失敗した：%v", err)
	}
	if got.CreatedAt.Before(before) {
		t.Errorf("作成時刻：got %v, want 保存した時刻（%v より後）", got.CreatedAt, before)
	}
}

// noclobber：同じメモに2回保存すると、先の要約が残り、エラーにならない
func TestSummary_SaveTwiceKeepsFirst(t *testing.T) {
	db := openTestDB(t)
	summaries := NewSummaryStore(db)
	first := newSavedNoteAndSummary(t, NewNoteStore(db), "- 先に保存した要約")
	second := first
	second.Text = "- 後から保存した要約"

	if err := summaries.Save(context.Background(), first); err != nil {
		t.Fatalf("1回目の保存に失敗した：%v", err)
	}
	if err := summaries.Save(context.Background(), second); err != nil {
		t.Errorf("2回目の保存がエラーになった：%v", err)
	}
	got, err := summaries.Get(context.Background(), first.NoteID)
	if err != nil {
		t.Fatalf("読み出しに失敗した：%v", err)
	}
	if got.Text != first.Text {
		t.Errorf("要約：got %q, want 先に保存した %q", got.Text, first.Text)
	}
}

// NXDOMAIN：メモ A の要約だけ保存して、メモ B を読むと「ない」
func TestSummary_GetOtherNoteNotFound(t *testing.T) {
	db := openTestDB(t)
	notes := NewNoteStore(db)
	summaries := NewSummaryStore(db)
	a := newSavedNoteAndSummary(t, notes, "- メモ A の要約")
	b := newSavedNoteAndSummary(t, notes, "（保存しない）")

	if err := summaries.Save(context.Background(), a); err != nil {
		t.Fatalf("保存に失敗した：%v", err)
	}
	_, err := summaries.Get(context.Background(), b.NoteID)
	if !errors.Is(err, summary.ErrNotFound) {
		t.Errorf("メモ B の要約のエラー：got %v, want summary.ErrNotFound", err)
	}
}
