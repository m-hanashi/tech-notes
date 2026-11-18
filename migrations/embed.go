// Package migrations は、DB のマイグレーションの SQL ファイルを、プログラムに埋め込む。
// 埋め込むので、イメージに SQL ファイルを別に置かなくて済む。
package migrations

import "embed"

// FS は、このフォルダの *.sql をまとめたもの。
//
//go:embed *.sql
var FS embed.FS
