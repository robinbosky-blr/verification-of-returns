package main

import (
	"context"
	"errors"
	"flag"
	"github.com/robinbosky-blr/verification-of-returns/backend/internal/server"
	"github.com/robinbosky-blr/verification-of-returns/backend/internal/storage"
	"github.com/robinbosky-blr/verification-of-returns/backend/internal/store"
	"github.com/robinbosky-blr/verification-of-returns/backend/internal/worker"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	mode := flag.String("mode", "all", "api, worker, migrate, or all")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		slog.Error("DATABASE_URL is required")
		os.Exit(1)
	}
	db, e := store.Open(ctx, url)
	if e != nil {
		slog.Error("database", "error", e)
		os.Exit(1)
	}
	defer db.Pool.Close()
	if *mode == "migrate" {
		return
	}
	files, e := storage.New()
	if e != nil {
		slog.Error("storage", "error", e)
		os.Exit(1)
	}
	w := worker.Worker{DB: db, Files: files}
	if *mode == "worker" {
		w.Run(ctx)
		return
	}
	if *mode == "all" {
		go w.Run(ctx)
	}
	password := os.Getenv("ADMIN_PASSWORD")
	if len(password) < 12 {
		slog.Error("ADMIN_PASSWORD must have at least 12 characters")
		os.Exit(1)
	}
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	s := server.Server{DB: db, Files: files, Password: password, Secure: os.Getenv("COOKIE_SECURE") != "false"}
	httpServer := &http.Server{Addr: addr, Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 5 * time.Minute, WriteTimeout: 5 * time.Minute, IdleTimeout: 60 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		httpServer.Shutdown(shutdown)
	}()
	slog.Info("returns API listening", "address", addr)
	if e = httpServer.ListenAndServe(); e != nil && !errors.Is(e, http.ErrServerClosed) {
		slog.Error("server", "error", e)
		os.Exit(1)
	}
}
