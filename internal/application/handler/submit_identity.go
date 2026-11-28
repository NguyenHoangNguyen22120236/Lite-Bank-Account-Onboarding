package handler

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	onboardingv1 "github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/gen/onboarding/v1"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/repository"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/service"
)

type SubmitIdentityService interface {
	SubmitIdentity(context.Context, string, *domain.Identity) (domain.Application, error)
}

func (h *Handler) SubmitIdentity(ctx context.Context, req *connect.Request[onboardingv1.SubmitIdentityRequest]) (*connect.Response[onboardingv1.SubmitIdentityResponse], error) {
	var identity *domain.Identity
	if input := req.Msg.GetIdentity(); input != nil {
		identity = &domain.Identity{IDNumber: input.GetIdNumber(), FullName: input.GetFullName(),
			DateOfBirth: input.GetDateOfBirth(), IDExpiryDate: input.GetIdExpiryDate(),
			PhoneNumber: input.GetPhoneNumber(), Email: input.Email}
	}
	app, err := h.submitIdentity.SubmitIdentity(ctx, req.Msg.GetApplicationId(), identity)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidApplicationID):
			return nil, connect.NewError(connect.CodeInvalidArgument, service.ErrInvalidApplicationID)
		case errors.Is(err, domain.ErrInvalidIdentity):
			return nil, connect.NewError(connect.CodeInvalidArgument, domain.ErrInvalidIdentity)
		case errors.Is(err, repository.ErrNotFound):
			return nil, connect.NewError(connect.CodeNotFound, repository.ErrNotFound)
		case errors.Is(err, domain.ErrConflictingIdentity):
			return nil, connect.NewError(connect.CodeAlreadyExists, domain.ErrConflictingIdentity)
		case errors.Is(err, domain.ErrInvalidIdentityState):
			return nil, connect.NewError(connect.CodeFailedPrecondition, domain.ErrInvalidIdentityState)
		case errors.Is(err, context.Canceled):
			return nil, connect.NewError(connect.CodeCanceled, errors.New("request canceled"))
		case errors.Is(err, context.DeadlineExceeded):
			return nil, connect.NewError(connect.CodeDeadlineExceeded, errors.New("request deadline exceeded"))
		default:
			return nil, connect.NewError(connect.CodeInternal, errors.New("could not submit identity"))
		}
	}
	status, ok := onboardingv1.ApplicationStatus_value[string(app.Status)]
	if !app.Status.Valid() || !ok {
		return nil, connect.NewError(connect.CodeInternal, errors.New("invalid application status"))
	}
	return connect.NewResponse(&onboardingv1.SubmitIdentityResponse{
		ApplicationId: app.ApplicationID.String(), Status: onboardingv1.ApplicationStatus(status),
	}), nil
}
