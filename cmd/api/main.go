// api は、Tech Notes の Web API を起動する。
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/m-hanashi/tech-notes/internal/httpapi"
	"github.com/m-hanashi/tech-notes/internal/postgres"
)

func main() {
	// ログは JSON で標準出力に出す（仕様書 第8章。CloudWatch Logs で項目ごとに検索できるようにするため）
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		slog.Error("環境変数 DATABASE_URL が空です")
		os.Exit(1)
	}
	// API キーが空のまま起動すると、誰でも使える状態になるので、起動しない。
	// 前後の空白と改行は取り除く（ファイルから登録した秘密情報の末尾に、改行が残ることがあるため）
	auth, err := httpapi.NewAPIKeyAuthenticator(strings.TrimSpace(os.Getenv("API_KEY")))
	if err != nil {
		slog.Error("環境変数 API_KEY が正しくありません", "error", err)
		os.Exit(1)
	}

	// ここでは DB に接続しない。最初のリクエストでつなぐ。
	// Aurora が自動休止していても、API の起動と /health は止めないため（仕様書 4.4、5.2）。
	db, err := postgres.Open(dsn)
	if err != nil {
		slog.Error("DB の接続の指定が正しくありません", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           httpapi.NewHandler(postgres.NewNoteStore(db), auth),
		ReadHeaderTimeout: 5 * time.Second,  // ヘッダーを送りきらない接続で、待たされ続けないようにする
		ReadTimeout:       10 * time.Second, // 本文をゆっくり送り続ける接続も切る
		WriteTimeout:      35 * time.Second, // DB を待つ上限（30秒）より長くする
		// ALB の待機の上限（60秒）より長くする。アプリが先に接続を切ると、ALB が切れた接続を使って 502 になるため
		IdleTimeout: 75 * time.Second,
	}

	// ECS がタスクを止めるときに送る SIGTERM を受け取ったら、
	// 処理中のリクエストを終えてから止まる
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("API を起動します", "port", port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("API の起動に失敗しました", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("API を止めます")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("API を止める処理に失敗しました", "error", err)
		os.Exit(1)
	}
}
