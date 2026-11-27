package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/repository"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestStartApplicationValidation(t *testing.T) {
	for _, input := range []string{
		"", "   ", "invalid", uuid.Nil.String(),
	} {
		t.Run(fmt.Sprintf("request_%q", input), func(t *testing.T) {
			repo := NewMockStartApplicationRepository(t)

			service := NewStartApplicationService(repo)

			got, err := service.StartApplication(
				context.Background(),
				input,
			)

			require.ErrorIs(t, err, ErrInvalidRequestID)
			require.Equal(t, domain.Application{}, got)
		})
	}
}

func TestStartApplicationCreation(t *testing.T) {
	requestID := uuid.New()
	ctx := context.Background()

	repo := NewMockStartApplicationRepository(t)

	var stored domain.Application

	repo.EXPECT().
		Create(ctx, mock.Anything).
		Run(func(_ context.Context, app domain.Application) {
			stored = app
		}).
		Return(nil).
		Once()

	service := NewStartApplicationService(repo)

	before := time.Now().UTC()
	got, err := service.StartApplication(ctx, requestID.String())
	after := time.Now().UTC()

	require.NoError(t, err)
	require.Equal(t, stored, got)

	require.NotEqual(t, uuid.Nil, got.ApplicationID)
	require.Equal(t, requestID, got.RequestID)

	require.Equal(t,
		domain.ApplicationStatusPendingTermsAcceptance,
		got.Status,
	)

	require.Equal(t,
		got.ApplicationID.String(),
		got.TemporalWorkflowID,
	)

	require.Nil(t, got.CompletedAt)
	require.Nil(t, got.RejectedAtStep)

	require.False(t, got.CreatedAt.Before(before))
	require.False(t, got.CreatedAt.After(after))
	require.Equal(t, time.UTC, got.CreatedAt.Location())
	require.Equal(t, got.CreatedAt, got.UpdatedAt)
}

func TestStartApplicationReplay(t *testing.T) {
	for _, duplicate := range []error{
		repository.ErrDuplicateRequestID,
		fmt.Errorf("create: %w", repository.ErrDuplicateRequestID),
	} {
		t.Run(duplicate.Error(), func(t *testing.T) {
			original, err := domain.NewApplication(
				uuid.New(),
				uuid.New(),
				time.Now(),
			)
			require.NoError(t, err)

			original.Status =
				domain.ApplicationStatusPendingIdentitySubmission

			repo := NewMockStartApplicationRepository(t)

			repo.EXPECT().
				Create(mock.Anything, mock.Anything).
				Return(duplicate).
				Once()

			repo.EXPECT().
				GetByRequestID(mock.Anything, original.RequestID).
				Return(original, nil).
				Once()

			service := NewStartApplicationService(repo)

			got, err := service.StartApplication(
				context.Background(),
				original.RequestID.String(),
			)

			require.NoError(t, err)
			require.Equal(t, original, got)
		})
	}
}

func TestStartApplicationErrorPropagation(t *testing.T) {
	for _, cause := range []error{
		repository.ErrDatabase,
		repository.ErrDuplicateApplicationID,
		repository.ErrDuplicateWorkflowID,
		repository.ErrNotFound,
		context.Canceled,
		context.DeadlineExceeded,
		errors.New("unexpected failure"),
	} {
		for _, lookup := range []bool{false, true} {
			t.Run(fmt.Sprintf("%v/lookup=%t", cause, lookup),
				func(t *testing.T) {
					repo := NewMockStartApplicationRepository(t)
					requestID := uuid.New()

					if lookup {
						repo.EXPECT().
							Create(mock.Anything, mock.Anything).
							Return(repository.ErrDuplicateRequestID).
							Once()

						repo.EXPECT().
							GetByRequestID(mock.Anything, requestID).
							Return(
								domain.Application{},
								fmt.Errorf("lookup: %w", cause),
							).
							Once()
					} else {
						repo.EXPECT().
							Create(mock.Anything, mock.Anything).
							Return(fmt.Errorf("create: %w", cause)).
							Once()
					}

					service := NewStartApplicationService(repo)

					got, err := service.StartApplication(
						context.Background(),
						requestID.String(),
					)

					require.ErrorIs(t, err, cause)
					require.Equal(t, domain.Application{}, got)
				})
		}
	}
}
