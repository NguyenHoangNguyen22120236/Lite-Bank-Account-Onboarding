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

type AcceptTermsService interface {
	AcceptTerms(context.Context, string, string) error
}

func (h *Handler) AcceptTerms(ctx context.Context, req *connect.Request[onboardingv1.AcceptTermsRequest]) (*connect.Response[onboardingv1.AcceptTermsResponse], error) {
	if err := h.acceptTerms.AcceptTerms(ctx, req.Msg.GetApplicationId(), req.Msg.GetTermId()); err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidApplicationID):
			return nil, connect.NewError(connect.CodeInvalidArgument, service.ErrInvalidApplicationID)
		case errors.Is(err, service.ErrInvalidTermID):
			return nil, connect.NewError(connect.CodeInvalidArgument, service.ErrInvalidTermID)
		case errors.Is(err, repository.ErrNotFound):
			return nil, connect.NewError(connect.CodeNotFound, repository.ErrNotFound)
		case errors.Is(err, domain.ErrConflictingTerms):
			return nil, connect.NewError(connect.CodeAlreadyExists, domain.ErrConflictingTerms)
		case errors.Is(err, domain.ErrInvalidTermsState):
			return nil, connect.NewError(connect.CodeFailedPrecondition, domain.ErrInvalidTermsState)
		case errors.Is(err, context.Canceled):
			return nil, connect.NewError(connect.CodeCanceled, errors.New("request canceled"))
		case errors.Is(err, context.DeadlineExceeded):
			return nil, connect.NewError(connect.CodeDeadlineExceeded, errors.New("request deadline exceeded"))
		default:
			return nil, connect.NewError(connect.CodeInternal, errors.New("could not accept terms"))
		}
	}
	return connect.NewResponse(&onboardingv1.AcceptTermsResponse{Accepted: true}), nil
}
