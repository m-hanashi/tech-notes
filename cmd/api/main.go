// api は、Tech Notes の Web API を起動する。
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/m-hanashi/tech-notes/internal/httpapi"
	"github.com/m-hanashi/tech-notes/internal/postgres"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		slog.Error("環境変数 DATABASE_URL が空です")
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
		Handler:           httpapi.NewRouter(postgres.NewNoteStore(db)),
		ReadHeaderTimeout: 5 * time.Second, // ヘッダーを送りきらない接続で、待たされ続けないようにする
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
