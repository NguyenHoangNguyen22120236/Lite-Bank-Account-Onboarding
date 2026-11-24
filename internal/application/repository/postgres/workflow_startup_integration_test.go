package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/repository"
)

func TestIntegrationWorkflowStartupConditionalTransition(t *testing.T) {
	pool, ctx := integrationPool(t)
	r := New(pool)
	for _, target := range []domain.ApplicationStatus{domain.ApplicationStatusPendingIdentityVerification, domain.ApplicationStatusFailedTechnical} {
		t.Run(string(target), func(t *testing.T) {
			app := fixture()
			app.Status = domain.ApplicationStatusPendingWorkflowStart
			if err := r.Create(ctx, app); err != nil {
				t.Fatal(err)
			}
			cleanupApplication(t, pool, app.ApplicationID)
			now := app.UpdatedAt.Add(time.Hour)
			got, err := r.ResolveWorkflowStart(ctx, app.ApplicationID, target, now)
			if err != nil {
				t.Fatal(err)
			}
			want := app
			want.Status, want.UpdatedAt = target, now
			assertApplication(t, got, want)
			stored, err := r.GetByID(ctx, app.ApplicationID)
			if err != nil {
				t.Fatal(err)
			}
			assertApplication(t, stored, want)
		})
	}
}

func TestIntegrationWorkflowStartupPreservesProgressedStatus(t *testing.T) {
	pool, ctx := integrationPool(t)
	r := New(pool)
	for _, initial := range []domain.ApplicationStatus{
		domain.ApplicationStatusPendingIdentityVerification,
		domain.ApplicationStatusPendingSanctionsScreening,
		domain.ApplicationStatusPendingHumanReview,
		domain.ApplicationStatusApproved,
		domain.ApplicationStatusPendingAccountCreation,
		domain.ApplicationStatusCompleted,
		domain.ApplicationStatusRejected,
		domain.ApplicationStatusFailedTechnical,
	} {
		t.Run(string(initial), func(t *testing.T) {
			app := fixture()
			app.Status = initial
			if initial == domain.ApplicationStatusCompleted || initial == domain.ApplicationStatusRejected {
				completed := app.UpdatedAt
				app.CompletedAt = &completed
			}
			if err := r.Create(ctx, app); err != nil {
				t.Fatal(err)
			}
			cleanupApplication(t, pool, app.ApplicationID)
			// One representative target is enough to check the pending-start guard.
			got, err := r.ResolveWorkflowStart(ctx, app.ApplicationID, domain.ApplicationStatusPendingIdentityVerification, app.UpdatedAt.Add(time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			assertApplication(t, got, app)
			stored, err := r.GetByID(ctx, app.ApplicationID)
			if err != nil {
				t.Fatal(err)
			}
			assertApplication(t, stored, app)
		})
	}
}

func TestIntegrationWorkflowStartupConcurrentOutcomes(t *testing.T) {
	pool, ctx := integrationPool(t)
	r := New(pool)
	app := fixture()
	app.Status = domain.ApplicationStatusPendingWorkflowStart
	if err := r.Create(ctx, app); err != nil {
		t.Fatal(err)
	}
	cleanupApplication(t, pool, app.ApplicationID)
	type result struct {
		app domain.Application
		err error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for index, status := range []domain.ApplicationStatus{domain.ApplicationStatusPendingIdentityVerification, domain.ApplicationStatusFailedTechnical} {
		wg.Add(1)
		go func(target domain.ApplicationStatus, now time.Time) {
			defer wg.Done()
			<-start
			got, err := r.ResolveWorkflowStart(ctx, app.ApplicationID, target, now)
			results <- result{got, err}
		}(status, app.UpdatedAt.Add(time.Duration(index+1)*time.Hour))
	}
	close(start)
	wg.Wait()
	close(results)
	stored, err := r.GetByID(ctx, app.ApplicationID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != domain.ApplicationStatusPendingIdentityVerification && stored.Status != domain.ApplicationStatusFailedTechnical {
		t.Fatal("neither startup outcome was persisted")
	}
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		assertApplication(t, result.app, stored)
	}
}

func TestWorkflowStartupTransitionDatabaseFailure(t *testing.T) {
	app := fixture()
	_, err := New(failingDB{err: errors.New("synthetic driver failure")}).ResolveWorkflowStart(context.Background(), app.ApplicationID, domain.ApplicationStatusPendingIdentityVerification, time.Now().UTC())
	if !errors.Is(err, repository.ErrDatabase) {
		t.Fatalf("database error classification lost: %v", err)
	}
}
