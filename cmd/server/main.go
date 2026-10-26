package main

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"libraryhub/internal/config"
	"libraryhub/internal/handler"
	"libraryhub/internal/repository"
	"libraryhub/internal/service"
	"libraryhub/migrations"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	startup, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(startup, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	for {
		err = pool.Ping(startup)
		if err == nil {
			break
		}
		select {
		case <-startup.Done():
			return startup.Err()
		case <-time.After(time.Second):
		}
	}
	if err = migrations.Apply(startup, pool); err != nil {
		return err
	}
	repo := repository.New(pool)
	srv := &http.Server{Addr: cfg.Addr, Handler: handler.Router(repo, service.New(repo), cfg.WebDir, cfg.DBTimeout), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	result := make(chan error, 1)
	go func() { slog.Info("LibraryHub ready", "addr", cfg.Addr); result <- srv.ListenAndServe() }()
	select {
	case err = <-result:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}
	shutdown, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	return srv.Shutdown(shutdown)
}
