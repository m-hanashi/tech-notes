// Package summary は、AI によるメモの要約をまとめる。
// Bedrock を呼び出す部品とプロンプトは、#29 でここに作る。
package summary

import "errors"

// ErrThrottled は、AI の呼び出し回数の制限を超えて、断られたことを表す。
var ErrThrottled = errors.New("AI の呼び出し回数の制限を超えました")
