package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/gen/onboarding/v1/onboardingv1connect"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/handler"
	applicationpostgres "github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/repository/postgres"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/service"
	applicationworkflow "github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/workflow"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/platform/config"
	platformpostgres "github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/platform/postgres"
	platformtemporal "github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/platform/temporal"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	if err := config.LoadDotEnv(".env"); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	startupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	pool, err := platformpostgres.Open(startupCtx, os.Getenv("DATABASE_URL"))
	cancel()
	if err != nil {
		return err
	}
	defer pool.Close()
	temporalConfig := platformtemporal.FromEnv()
	temporalCtx, temporalCancel := context.WithTimeout(ctx, 10*time.Second)
	temporalClient, err := platformtemporal.Open(temporalCtx, temporalConfig)
	temporalCancel()
	if err != nil {
		return err
	}
	defer temporalClient.Close()

	mux := http.NewServeMux()
	applicationRepository := applicationpostgres.NewApplicationRepository(pool)
	transactor := applicationpostgres.NewTransactor(pool)
	applicationHandler := handler.New(
		service.NewStartApplicationService(applicationRepository, applicationworkflow.New(temporalClient, temporalConfig.TaskQueue)),
		service.NewAcceptTermsService(transactor, applicationRepository),
		service.NewSubmitIdentityService(transactor, applicationRepository),
	)

	path, rpcHandler :=
		onboardingv1connect.NewApplicationServiceHandler(applicationHandler)

	mux.Handle(path, rpcHandler)

	mux.HandleFunc("/health", func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("application service healthy"))
	})

	addr := ":8080"

	log.Printf("ApplicationService running on %s", addr)

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second, // Prevent Slowloris attack
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.ListenAndServe() }()
	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return err
		}
		return nil
	}
}
