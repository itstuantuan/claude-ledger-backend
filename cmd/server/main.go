package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"cloud-ledger-backend/internal/platform/config"
	"cloud-ledger-backend/internal/platform/database"
	"cloud-ledger-backend/internal/platform/logger"
	"cloud-ledger-backend/internal/platform/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	log, err := logger.New(cfg.AppEnv, cfg.LogLevel)
	if err != nil {
		panic(err)
	}
	defer func() { _ = log.Sync() }()

	db, sqlDB, err := database.Open(cfg.Database)
	if err != nil {
		log.Fatal("database connection failed", logger.Error(err))
	}
	defer func() { _ = sqlDB.Close() }()

	handler := server.NewRouter(cfg, log, db)
	httpServer := &http.Server{
		Addr:              ":" + cfg.AppPort,
		Handler:           handler,
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Info("http server started", logger.String("address", httpServer.Addr), logger.String("environment", cfg.AppEnv))
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-signals:
		log.Info("shutdown signal received", logger.String("signal", sig.String()))
	case err := <-serverErr:
		log.Error("http server failed", logger.Error(err))
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Error("graceful shutdown failed", logger.Error(err))
	}
}
