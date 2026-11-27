package service

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/repository"
)

// startupFunc supplies controlled Temporal outcomes through the service port.
type startupFunc func(context.Context, string, string) (WorkflowStartOutcome, error)

func (f startupFunc) EnsureStarted(ctx context.Context, applicationID, workflowID string) (WorkflowStartOutcome, error) {
	return f(ctx, applicationID, workflowID)
}

type startupRepository struct {
	app                                 domain.Application
	submitErr, transitionErr, lookupErr error
	persisted                           bool
	inTransaction                       bool
	transactionCalls                    int
	transitions, lookups                int
	beforeTransition                    func(*domain.Application)
}

func (r *startupRepository) WithinTx(ctx context.Context, fn func(context.Context) error) error {
	r.transactionCalls++
	r.inTransaction = true
	defer func() { r.inTransaction = false }()
	if err := fn(ctx); err != nil {
		return err
	}
	r.persisted = true
	return nil
}

func (r *startupRepository) GetByIDForUpdate(context.Context, uuid.UUID) (domain.Application, error) {
	if r.transactionCalls == 1 && r.submitErr != nil {
		return domain.Application{}, r.submitErr
	}
	if r.transactionCalls > 1 && r.beforeTransition != nil {
		r.beforeTransition(&r.app)
	}
	return r.app, nil
}

func (r *startupRepository) GetIdentity(context.Context, uuid.UUID) (*domain.Identity, error) {
	i := serviceIdentity()
	i.ApplicationID = r.app.ApplicationID
	return i, nil
}

func (r *startupRepository) InsertIdentity(context.Context, domain.Identity) error {
	panic("unexpected identity insert")
}

func (r *startupRepository) AdvanceAfterIdentity(context.Context, domain.Identity) (domain.Application, error) {
	panic("unexpected identity transition")
}

func (r *startupRepository) GetByID(context.Context, uuid.UUID) (domain.Application, error) {
	r.lookups++
	return r.app, r.lookupErr
}

func (r *startupRepository) ResolveWorkflowStart(_ context.Context, id uuid.UUID, target domain.ApplicationStatus, now time.Time) (domain.Application, error) {
	r.transitions++
	if r.transitionErr != nil {
		return domain.Application{}, r.transitionErr
	}
	if r.app.ApplicationID != id {
		panic("wrong application ID")
	}
	if r.app.Status == domain.ApplicationStatusPendingWorkflowStart {
		r.app.Status, r.app.UpdatedAt = target, now
	}
	return r.app, nil
}

func startupFixture() *startupRepository {
	id := uuid.New()
	return &startupRepository{
		app: domain.Application{
			ApplicationID:      id,
			TemporalWorkflowID: id.String(),
			Status:             domain.ApplicationStatusPendingWorkflowStart,
			UpdatedAt:          time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}
}

func TestWorkflowStartupOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name           string
		outcome        WorkflowStartOutcome
		cause, wantErr error
		wantStatus     domain.ApplicationStatus
		writes         int
	}{
		{"accepted or running", WorkflowStartEstablished, nil, nil, domain.ApplicationStatusPendingIdentityVerification, 1},
		{"closed unsuccessfully", WorkflowStartClosedUnsuccessfully, nil, ErrWorkflowClosed, domain.ApplicationStatusFailedTechnical, 1},
		{"completed with pending application", WorkflowStartCompleted, nil, ErrWorkflowStateInconsistent, domain.ApplicationStatusPendingWorkflowStart, 0},
		{"uncertain timeout", 0, context.DeadlineExceeded, context.DeadlineExceeded, domain.ApplicationStatusPendingWorkflowStart, 0},
		{"unavailable", 0, ErrWorkflowUnavailable, ErrWorkflowUnavailable, domain.ApplicationStatusPendingWorkflowStart, 0},
		{"canceled", 0, context.Canceled, context.Canceled, domain.ApplicationStatusPendingWorkflowStart, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := startupFixture()
			before := r.app
			calls := 0
			starter := startupFunc(func(_ context.Context, appID, workflowID string) (WorkflowStartOutcome, error) {
				calls++
				if !r.persisted || r.inTransaction || appID != r.app.ApplicationID.String() || workflowID != before.TemporalWorkflowID {
					t.Fatal("startup must follow persistence and use stored IDs")
				}
				return tc.outcome, tc.cause
			})
			got, err := NewSubmitIdentityService(r, r, starter).SubmitIdentity(context.Background(), r.app.ApplicationID.String(), serviceIdentity())
			if !errors.Is(err, tc.wantErr) || calls != 1 || r.transitions != tc.writes || r.app.Status != tc.wantStatus {
				t.Fatalf("error=%v calls=%d transitions=%d status=%s", err, calls, r.transitions, r.app.Status)
			}
			if err == nil && !reflect.DeepEqual(got, r.app) {
				t.Fatal("response must use persisted application")
			}
			if tc.writes == 0 && !reflect.DeepEqual(before, r.app) {
				t.Fatal("failure changed persisted application")
			}
		})
	}
}

func TestWorkflowStartupPersistenceFailure(t *testing.T) {
	r := startupFixture()
	r.submitErr = repository.ErrDatabase
	starter := startupFunc(func(context.Context, string, string) (WorkflowStartOutcome, error) {
		t.Fatal("startup before successful persistence")
		return 0, nil
	})
	_, err := NewSubmitIdentityService(r, r, starter).SubmitIdentity(context.Background(), r.app.ApplicationID.String(), serviceIdentity())
	if !errors.Is(err, repository.ErrDatabase) || r.transitions != 0 {
		t.Fatalf("unexpected result: %v", err)
	}
}

func TestWorkflowStartupReplaySkipsTemporal(t *testing.T) {
	for _, status := range []domain.ApplicationStatus{domain.ApplicationStatusPendingIdentityVerification, domain.ApplicationStatusPendingSanctionsScreening, domain.ApplicationStatusPendingHumanReview, domain.ApplicationStatusApproved, domain.ApplicationStatusPendingAccountCreation, domain.ApplicationStatusCompleted, domain.ApplicationStatusRejected, domain.ApplicationStatusFailedTechnical} {
		t.Run(string(status), func(t *testing.T) {
			r := startupFixture()
			r.app.Status = status
			before := r.app
			starter := startupFunc(func(context.Context, string, string) (WorkflowStartOutcome, error) {
				t.Fatal("progressed replay started Temporal")
				return 0, nil
			})
			got, err := NewSubmitIdentityService(r, r, starter).SubmitIdentity(context.Background(), r.app.ApplicationID.String(), serviceIdentity())
			if err != nil || !reflect.DeepEqual(got, before) || r.transitions != 0 {
				t.Fatalf("replay changed application: %+v %v", got, err)
			}
		})
	}
}

func TestWorkflowStartupRetryAfterUncertainOrDatabaseFailure(t *testing.T) {
	for _, databaseFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "lost acknowledgement", true: "post-start database failure"}[databaseFailure], func(t *testing.T) {
			r := startupFixture()
			originalID := r.app.TemporalWorkflowID
			calls := 0
			starter := startupFunc(func(_ context.Context, _, workflowID string) (WorkflowStartOutcome, error) {
				calls++
				if workflowID != originalID {
					t.Fatal("retry changed workflow ID")
				}
				if calls == 1 && !databaseFailure {
					return 0, context.DeadlineExceeded
				}
				return WorkflowStartEstablished, nil
			})
			if databaseFailure {
				r.transitionErr = repository.ErrDatabase
			}
			s := NewSubmitIdentityService(r, r, starter)
			_, err := s.SubmitIdentity(context.Background(), r.app.ApplicationID.String(), serviceIdentity())
			if err == nil || calls != 1 || !r.persisted || r.app.Status != domain.ApplicationStatusPendingWorkflowStart {
				t.Fatal("first failure must retain identity and pending status without retrying")
			}
			r.transitionErr = nil
			got, err := s.SubmitIdentity(context.Background(), r.app.ApplicationID.String(), serviceIdentity())
			if err != nil || calls != 2 || got.Status != domain.ApplicationStatusPendingIdentityVerification {
				t.Fatalf("retry failed: %+v %v calls=%d", got, err, calls)
			}
		})
	}
}

func TestWorkflowStartupConcurrentProgressWins(t *testing.T) {
	for _, outcome := range []WorkflowStartOutcome{WorkflowStartEstablished, WorkflowStartClosedUnsuccessfully} {
		r := startupFixture()
		r.beforeTransition = func(app *domain.Application) { app.Status = domain.ApplicationStatusCompleted }
		starter := startupFunc(func(context.Context, string, string) (WorkflowStartOutcome, error) { return outcome, nil })
		got, err := NewSubmitIdentityService(r, r, starter).SubmitIdentity(context.Background(), r.app.ApplicationID.String(), serviceIdentity())
		if err != nil || got.Status != domain.ApplicationStatusCompleted {
			t.Fatalf("concurrent progress overwritten: %+v %v", got, err)
		}
	}
}

func TestWorkflowStartupCompletedReconcilesPersistedState(t *testing.T) {
	for _, status := range []domain.ApplicationStatus{domain.ApplicationStatusCompleted, domain.ApplicationStatusRejected, domain.ApplicationStatusPendingIdentityVerification} {
		t.Run(string(status), func(t *testing.T) {
			r := startupFixture()
			starter := startupFunc(func(context.Context, string, string) (WorkflowStartOutcome, error) {
				r.app.Status = status // another writer progresses after the identity transaction
				return WorkflowStartCompleted, nil
			})
			got, err := NewSubmitIdentityService(r, r, starter).SubmitIdentity(context.Background(), r.app.ApplicationID.String(), serviceIdentity())
			if r.lookups != 1 || r.transitions != 0 {
				t.Fatal("completed execution must reconcile with a fresh read and no writes")
			}
			if status == domain.ApplicationStatusPendingIdentityVerification {
				if !errors.Is(err, ErrWorkflowStateInconsistent) {
					t.Fatalf("inconsistent completion accepted: %v", err)
				}
			} else if err != nil || got.Status != status {
				t.Fatalf("terminal state lost: %+v %v", got, err)
			}
		})
	}
}

func TestWorkflowStartupRejectsInconsistentStoredID(t *testing.T) {
	for _, workflowID := range []string{"", uuid.NewString()} {
		r := startupFixture()
		r.app.TemporalWorkflowID = workflowID
		starter := startupFunc(func(context.Context, string, string) (WorkflowStartOutcome, error) {
			t.Fatal("inconsistent persisted workflow ID reached Temporal")
			return 0, nil
		})
		_, err := NewSubmitIdentityService(r, r, starter).SubmitIdentity(context.Background(), r.app.ApplicationID.String(), serviceIdentity())
		if !errors.Is(err, ErrWorkflowStateInconsistent) || r.transitions != 0 {
			t.Fatalf("stored workflow ID was silently replaced: %v", err)
		}
	}
}

func TestWorkflowStartupReconciliationReadFailure(t *testing.T) {
	r := startupFixture()
	r.lookupErr = repository.ErrDatabase
	starter := startupFunc(func(context.Context, string, string) (WorkflowStartOutcome, error) {
		return WorkflowStartCompleted, nil
	})
	_, err := NewSubmitIdentityService(r, r, starter).SubmitIdentity(context.Background(), r.app.ApplicationID.String(), serviceIdentity())
	if !errors.Is(err, repository.ErrDatabase) || r.lookups != 1 || r.transitions != 0 || r.app.Status != domain.ApplicationStatusPendingWorkflowStart {
		t.Fatalf("failed reconciliation must leave application pending: %v", err)
	}
}
