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
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           httpapi.NewRouter(nil), // メモの API は、DB につなぐ次の PR で有効にする
		ReadHeaderTimeout: 5 * time.Second,        // ヘッダーを送りきらない接続で、待たされ続けないようにする
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
