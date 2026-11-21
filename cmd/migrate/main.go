// migrate は、DB のマイグレーションを実行する（仕様書 7.4）。
//
//	go run ./cmd/migrate         まだ適用していないマイグレーションを、番号の順に適用する
//	go run ./cmd/migrate status  どのマイグレーションを適用済みかを表示する
//
// 接続先は、環境変数 DATABASE_URL で渡す。
// 失敗したら終了コード1で止まる。段階5の CD は、この終了コードで成否を判定する。
package main

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL のドライバー（pgx）を "pgx" という名前で登録する
	"github.com/pressly/goose/v3"

	"github.com/m-hanashi/tech-notes/migrations"
)

func main() {
	// ログは JSON で標準出力に出す（cmd/api と同じ）
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	if err := run(); err != nil {
		slog.Error("マイグレーションに失敗しました", "error", err)
		os.Exit(1)
	}
}

func run() error {
	command := "up"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("環境変数 DATABASE_URL が空です")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}

	switch command {
	case "up":
		return goose.Up(db, ".")
	case "status":
		return goose.Status(db, ".")
	default:
		return fmt.Errorf("知らないコマンドです：%s（up か status を指定する）", command)
	}
}
