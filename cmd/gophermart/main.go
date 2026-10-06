package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/EzhiG/gophermart-diploma/internal/config"
	"github.com/EzhiG/gophermart-diploma/internal/handler"
	"github.com/EzhiG/gophermart-diploma/internal/logger"
	"github.com/EzhiG/gophermart-diploma/internal/repository"
	"github.com/go-chi/chi/v5"
)

func main() {
	sugar, err := logger.New()
	if err != nil {
		sugar.Fatal(err)
	}
	defer sugar.Sync()

	cfg := config.New()

	storage, err := repository.NewPGStorage(cfg.DatabaseURI)
	if err != nil {
		sugar.Fatal(err)
	}

	defer storage.Close()
	loggerMw := logger.NewMiddleware(sugar)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	router := chi.NewRouter()
	router.Use(loggerMw, handler.MaxBytesMiddleware, handler.GzipMiddleware)

	srv := &http.Server{Addr: cfg.RunAddress, Handler: router}

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			sugar.Fatal(err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = srv.Shutdown(shutdownCtx)
	if err != nil {
		sugar.Error(err)
	}
}
