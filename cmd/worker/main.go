package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/platform/config"
	platformtemporal "github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/platform/temporal"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/workflow"
	"go.temporal.io/sdk/worker"
	temporalworkflow "go.temporal.io/sdk/workflow"
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
	cfg := platformtemporal.FromEnv()
	startupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	c, err := platformtemporal.Open(startupCtx, cfg)
	cancel()
	if err != nil {
		return err
	}
	defer c.Close()
	w := worker.New(c, cfg.TaskQueue, worker.Options{WorkerStopTimeout: 10 * time.Second})
	w.RegisterWorkflowWithOptions(workflow.OnboardingWorkflow, temporalworkflow.RegisterOptions{Name: workflow.Name})
	if err := w.Start(); err != nil {
		return err
	}
	defer w.Stop()
	log.Printf("Onboarding worker polling task queue %s", cfg.TaskQueue)
	<-ctx.Done()
	return nil
}
