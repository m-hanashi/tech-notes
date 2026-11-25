// Package summary は、AI によるメモの要約をまとめる。
// Bedrock を呼び出す部品とプロンプトは、#29 でここに作る。
package summary

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrThrottled は、AI の呼び出し回数の制限を超えて、断られたことを表す。
var ErrThrottled = errors.New("AI の呼び出し回数の制限を超えました")

// ErrNotFound は、そのメモの要約が、まだ保存されていないことを表す。
var ErrNotFound = errors.New("要約が保存されていません")

// Generated は、AI が作った要約と、そのときに使ったモデルとトークン数。
type Generated struct {
	Text         string
	Model        string
	InputTokens  int
	OutputTokens int
}

// Summary は、保存した要約1件（仕様書 4.4）。メモ1件につき1件。
type Summary struct {
	NoteID       uuid.UUID
	Text         string
	Model        string        // 要約に使ったモデル
	InputTokens  int           // AI に渡したトークン数
	OutputTokens int           // AI が返したトークン数
	Latency      time.Duration // AI の呼び出しにかかった時間
	CreatedAt    time.Time
}
