package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/EzhiG/gophermart-diploma/internal/auth"
	"github.com/EzhiG/gophermart-diploma/internal/config"
	"github.com/EzhiG/gophermart-diploma/internal/gophermart"
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
	authSvc := auth.New(cfg.SecretKey, cfg.TokenExpirationTime, storage)
	martSvc := gophermart.New(storage, cfg.AccrualSystemAddress, sugar)
	h := handler.New(authSvc, martSvc, sugar)
	loggerMw := logger.NewMiddleware(sugar)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go martSvc.RunAccrualSync(ctx)

	router := chi.NewRouter()
	router.Use(loggerMw, handler.MaxBytesMiddleware, h.GzipMiddleware)
	router.Post("/api/user/register", h.APIPostUserRegister)
	router.Post("/api/user/login", h.APIPostUserLogin)
	router.Group(func(r chi.Router) {
		r.Use(h.NewAuthMiddleware())
		r.Post("/api/user/orders", h.APIPostUserOrder)
		r.Get("/api/user/orders", h.APIGetUserOrders)
		r.Get("/api/user/balance", h.APIGetUserBalance)
		r.Post("/api/user/balance/withdraw", h.APIPostUserWithdraw)
		r.Get("/api/user/withdrawals", h.APIGetUserWithdrawals)
	})
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
	martSvc.WaitAccrualSync()
}
