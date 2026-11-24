package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/repository"
)

type repositoryStub struct {
	create func(context.Context, domain.Application) error
	get    func(context.Context, uuid.UUID) (domain.Application, error)
}

func (r repositoryStub) SubmitIdentity(context.Context, domain.Identity) (domain.Application, error) {
	panic("unexpected SubmitIdentity call")
}

func (r repositoryStub) AcceptTerms(context.Context, domain.TermsAcceptance) error {
	panic("unexpected AcceptTerms call")
}

func (r repositoryStub) Create(ctx context.Context, app domain.Application) error {
	return r.create(ctx, app)
}

func (r repositoryStub) GetByRequestID(ctx context.Context, id uuid.UUID) (domain.Application, error) {
	return r.get(ctx, id)
}

func TestStartApplicationValidation(t *testing.T) {
	for _, input := range []string{"", "   ", "invalid", uuid.Nil.String()} {
		t.Run(fmt.Sprintf("request_%q", input), func(t *testing.T) {
			r := repositoryStub{
				create: func(context.Context, domain.Application) error { t.Fatal("unexpected Create"); return nil },
				get: func(context.Context, uuid.UUID) (domain.Application, error) {
					t.Fatal("unexpected GetByRequestID")
					return domain.Application{}, nil
				},
			}
			got, err := New(r).StartApplication(context.Background(), input)
			if !errors.Is(err, ErrInvalidRequestID) || !reflect.DeepEqual(got, domain.Application{}) {
				t.Fatalf("got %+v, %v; want zero application and ErrInvalidRequestID", got, err)
			}
		})
	}
}

func TestStartApplicationCreation(t *testing.T) {
	requestID := uuid.New()
	var stored domain.Application
	calls := 0
	ctx := context.Background()
	r := repositoryStub{create: func(gotCtx context.Context, app domain.Application) error {
		if gotCtx != ctx {
			t.Fatal("request context was not forwarded")
		}
		calls++
		stored = app
		return nil
	}}
	before := time.Now().UTC()
	got, err := New(r).StartApplication(ctx, requestID.String())
	after := time.Now().UTC()
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !reflect.DeepEqual(got, stored) {
		t.Fatal("expected one Create and its application as the result")
	}
	if got.ApplicationID == uuid.Nil || got.RequestID != requestID ||
		got.Status != domain.ApplicationStatusPendingTermsAcceptance ||
		got.TemporalWorkflowID != got.ApplicationID.String() ||
		got.CompletedAt != nil || got.RejectedAtStep != nil {
		t.Fatalf("unexpected application defaults: %+v", got)
	}
	if got.CreatedAt.Before(before) || got.CreatedAt.After(after) ||
		got.CreatedAt.Location() != time.UTC || got.UpdatedAt != got.CreatedAt {
		t.Fatalf("unexpected timestamps: %+v", got)
	}
}

func TestStartApplicationReplay(t *testing.T) {
	for _, duplicate := range []error{repository.ErrDuplicateRequestID, fmt.Errorf("create: %w", repository.ErrDuplicateRequestID)} {
		t.Run(duplicate.Error(), func(t *testing.T) {
			original, err := domain.NewApplication(uuid.New(), uuid.New(), time.Now())
			if err != nil {
				t.Fatal(err)
			}
			original.Status = domain.ApplicationStatusPendingIdentitySubmission
			creates, lookups := 0, 0
			r := repositoryStub{
				create: func(context.Context, domain.Application) error { creates++; return duplicate },
				get: func(_ context.Context, id uuid.UUID) (domain.Application, error) {
					lookups++
					if id != original.RequestID {
						t.Fatal("lookup used the wrong request ID")
					}
					return original, nil
				},
			}
			got, err := New(r).StartApplication(context.Background(), original.RequestID.String())
			if err != nil || creates != 1 || lookups != 1 || !reflect.DeepEqual(got, original) {
				t.Fatalf("replay got %+v, %v; create=%d lookup=%d", got, err, creates, lookups)
			}
		})
	}
}

func TestStartApplicationErrorPropagation(t *testing.T) {
	for _, cause := range []error{
		repository.ErrDatabase, repository.ErrDuplicateApplicationID, repository.ErrDuplicateWorkflowID,
		repository.ErrNotFound, context.Canceled, context.DeadlineExceeded, errors.New("unexpected failure"),
	} {
		for _, lookup := range []bool{false, true} {
			t.Run(fmt.Sprintf("%v/lookup=%t", cause, lookup), func(t *testing.T) {
				lookups := 0
				r := repositoryStub{
					create: func(context.Context, domain.Application) error {
						if lookup {
							return repository.ErrDuplicateRequestID
						}
						return fmt.Errorf("create: %w", cause)
					},
					get: func(context.Context, uuid.UUID) (domain.Application, error) {
						lookups++
						return domain.Application{}, fmt.Errorf("lookup: %w", cause)
					},
				}
				got, err := New(r).StartApplication(context.Background(), uuid.NewString())
				if !errors.Is(err, cause) || !reflect.DeepEqual(got, domain.Application{}) {
					t.Fatalf("got %+v, %v; want zero application and %v", got, err, cause)
				}
				if (lookup && lookups != 1) || (!lookup && lookups != 0) {
					t.Fatalf("unexpected lookup count: %d", lookups)
				}
			})
		}
	}
}
