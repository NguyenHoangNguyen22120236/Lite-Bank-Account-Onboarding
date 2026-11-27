package service

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/repository"
	"github.com/google/uuid"
)

type transactionTestKey struct{}

type transactorStub struct {
	within func(context.Context, func(context.Context) error) error
}

func (s transactorStub) WithinTx(ctx context.Context, fn func(context.Context) error) error {
	return s.within(ctx, fn)
}

// orchestrationFixture checks that every persistence operation uses the callback context.
type orchestrationFixture struct {
	t                *testing.T
	app              domain.Application
	identity         *domain.Identity
	terms            []string
	events           []string
	failAt           string
	cause            error
	insertedIdentity *domain.Identity
	insertedTerms    *domain.TermsAcceptance
	committed        bool
}

func newOrchestrationFixture(t *testing.T, status domain.ApplicationStatus) *orchestrationFixture {
	id := uuid.New()
	return &orchestrationFixture{t: t, app: domain.Application{
		ApplicationID: id, TemporalWorkflowID: id.String(), Status: status,
		UpdatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}, cause: repository.ErrDatabase}
}

func (f *orchestrationFixture) step(ctx context.Context, name string) error {
	f.t.Helper()
	if ctx.Value(transactionTestKey{}) != f {
		f.t.Fatalf("%s did not use transaction context", name)
	}
	f.events = append(f.events, name)
	if f.failAt == name {
		return f.cause
	}
	return nil
}

func (f *orchestrationFixture) WithinTx(ctx context.Context, fn func(context.Context) error) error {
	f.events = append(f.events, "begin")
	if f.failAt == "begin" {
		return f.cause
	}
	app, identity, terms := f.app, f.identity, append([]string(nil), f.terms...)
	err := fn(context.WithValue(ctx, transactionTestKey{}, f))
	if err == nil {
		f.events = append(f.events, "commit")
		if f.failAt == "commit" {
			err = f.cause
		}
	}
	if err != nil {
		f.app, f.identity, f.terms = app, identity, terms
		f.events = append(f.events, "rollback")
		return err
	}
	f.committed = true
	return nil
}

func (f *orchestrationFixture) GetByIDForUpdate(ctx context.Context, id uuid.UUID) (domain.Application, error) {
	if err := f.step(ctx, "lock"); err != nil {
		return domain.Application{}, err
	}
	if id != f.app.ApplicationID {
		f.t.Fatal("wrong application ID")
	}
	return f.app, nil
}

func (f *orchestrationFixture) GetByID(ctx context.Context, id uuid.UUID) (domain.Application, error) {
	return f.app, nil
}

func (f *orchestrationFixture) AdvanceAfterTerms(ctx context.Context, a domain.TermsAcceptance) error {
	if err := f.step(ctx, "advance terms"); err != nil {
		return err
	}
	if f.insertedTerms == nil || !reflect.DeepEqual(*f.insertedTerms, a) {
		f.t.Fatal("terms status update must use inserted acceptance metadata")
	}
	f.app.Status, f.app.UpdatedAt = domain.ApplicationStatusPendingIdentitySubmission, a.AcceptedAt
	return nil
}

func (f *orchestrationFixture) AdvanceAfterIdentity(ctx context.Context, i domain.Identity) (domain.Application, error) {
	if err := f.step(ctx, "advance identity"); err != nil {
		return domain.Application{}, err
	}
	if f.insertedIdentity == nil || !reflect.DeepEqual(*f.insertedIdentity, i) {
		f.t.Fatal("identity status update must use inserted identity metadata")
	}
	f.app.Status, f.app.UpdatedAt = domain.ApplicationStatusPendingWorkflowStart, i.CreatedAt
	return f.app, nil
}

func (f *orchestrationFixture) ResolveWorkflowStart(ctx context.Context, id uuid.UUID, target domain.ApplicationStatus, now time.Time) (domain.Application, error) {
	if err := f.step(ctx, "advance workflow"); err != nil {
		return domain.Application{}, err
	}
	f.app.Status, f.app.UpdatedAt = target, now
	return f.app, nil
}

func (f *orchestrationFixture) GetTermIDs(ctx context.Context, id uuid.UUID) ([]string, error) {
	if err := f.step(ctx, "read terms"); err != nil {
		return nil, err
	}
	return f.terms, nil
}
func (f *orchestrationFixture) InsertTermsAcceptance(ctx context.Context, a domain.TermsAcceptance) error {
	if err := f.step(ctx, "insert terms"); err != nil {
		return err
	}
	f.insertedTerms = &a
	f.terms = append(f.terms, a.TermID)
	return nil
}

func (f *orchestrationFixture) GetIdentity(ctx context.Context, id uuid.UUID) (*domain.Identity, error) {
	if err := f.step(ctx, "read identity"); err != nil {
		return nil, err
	}
	return f.identity, nil
}
func (f *orchestrationFixture) InsertIdentity(ctx context.Context, i domain.Identity) error {
	if err := f.step(ctx, "insert identity"); err != nil {
		return err
	}
	f.insertedIdentity, f.identity = &i, &i
	return nil
}

func noTransaction(t *testing.T) transactorStub {
	return transactorStub{within: func(context.Context, func(context.Context) error) error {
		t.Fatal("invalid input started a transaction")
		return errors.New("unexpected transaction")
	}}
}

var (
	_ AcceptTermsRepository    = (*orchestrationFixture)(nil)
	_ SubmitIdentityRepository = (*orchestrationFixture)(nil)
)
