// Package notes は、メモそのものと、メモの決まり（文字数など）をまとめる。
package notes

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// 文字数の上限（仕様書 4.4）。DB の CHECK 制約（migrations/00001_create_notes.sql）と同じ値にする。
const (
	MaxTitleLength = 200
	MaxBodyLength  = 10000
)

// ErrNotFound は、指定したメモがないことを表す。
var ErrNotFound = errors.New("メモが見つかりません")

// Note は、メモ1件。
type Note struct {
	ID        uuid.UUID `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// Validate は、title と body が決まりを満たすかを確かめる。
// 文字数は、バイト数ではなく文字数で数える（DB の char_length と同じ数え方）。
func Validate(title, body string) error {
	if err := validateText("title", title, MaxTitleLength); err != nil {
		return err
	}
	return validateText("body", body, MaxBodyLength)
}

func validateText(name, value string, maxLength int) error {
	if strings.TrimSpace(value) == "" {
		return errors.New(name + " は必須です")
	}
	if utf8.RuneCountInString(value) > maxLength {
		return fmt.Errorf("%s は%d文字以内にしてください", name, maxLength)
	}
	return nil
}
