package handler

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	onboardingv1 "github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/gen/onboarding/v1"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/gen/onboarding/v1/onboardingv1connect"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/repository"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/service"
)

type Service interface {
	SubmitIdentity(context.Context, string, *domain.Identity) (domain.Application, error)
	AcceptTerms(context.Context, string, string) error
	StartApplication(context.Context, string) (domain.Application, error)
}

type Handler struct {
	service Service
}

var _ onboardingv1connect.ApplicationServiceHandler = (*Handler)(nil)

func New(service Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) AcceptTerms(ctx context.Context, req *connect.Request[onboardingv1.AcceptTermsRequest]) (*connect.Response[onboardingv1.AcceptTermsResponse], error) {
	if err := h.service.AcceptTerms(ctx, req.Msg.GetApplicationId(), req.Msg.GetTermId()); err != nil {
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

func (h *Handler) SubmitIdentity(ctx context.Context, req *connect.Request[onboardingv1.SubmitIdentityRequest]) (*connect.Response[onboardingv1.SubmitIdentityResponse], error) {
	var identity *domain.Identity
	if input := req.Msg.GetIdentity(); input != nil {
		identity = &domain.Identity{IDNumber: input.GetIdNumber(), FullName: input.GetFullName(),
			DateOfBirth: input.GetDateOfBirth(), IDExpiryDate: input.GetIdExpiryDate(),
			PhoneNumber: input.GetPhoneNumber(), Email: input.Email}
	}
	app, err := h.service.SubmitIdentity(ctx, req.Msg.GetApplicationId(), identity)
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

func (h *Handler) ReviewApplication(context.Context, *connect.Request[onboardingv1.ReviewApplicationRequest]) (*connect.Response[onboardingv1.ReviewApplicationResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("onboarding.v1.ApplicationService.ReviewApplication is not implemented"))
}

func (h *Handler) GetApplication(context.Context, *connect.Request[onboardingv1.GetApplicationRequest]) (*connect.Response[onboardingv1.GetApplicationResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("onboarding.v1.ApplicationService.GetApplication is not implemented"))
}

func (h *Handler) UpdateApplicationStatus(context.Context, *connect.Request[onboardingv1.UpdateApplicationStatusRequest]) (*connect.Response[onboardingv1.UpdateApplicationStatusResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("onboarding.v1.ApplicationService.UpdateApplicationStatus is not implemented"))
}

func (h *Handler) StartApplication(ctx context.Context, req *connect.Request[onboardingv1.StartApplicationRequest]) (*connect.Response[onboardingv1.StartApplicationResponse], error) {
	app, err := h.service.StartApplication(ctx, req.Msg.GetRequestId())
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidRequestID):
			return nil, connect.NewError(connect.CodeInvalidArgument, service.ErrInvalidRequestID)
		case errors.Is(err, context.Canceled):
			return nil, connect.NewError(connect.CodeCanceled, errors.New("request canceled"))
		case errors.Is(err, context.DeadlineExceeded):
			return nil, connect.NewError(connect.CodeDeadlineExceeded, errors.New("request deadline exceeded"))
		default:
			return nil, connect.NewError(connect.CodeInternal, errors.New("could not start application"))
		}
	}
	status, ok := onboardingv1.ApplicationStatus_value[string(app.Status)]
	if !app.Status.Valid() || !ok {
		return nil, connect.NewError(connect.CodeInternal, errors.New("invalid application status"))
	}
	return connect.NewResponse(&onboardingv1.StartApplicationResponse{
		ApplicationId: app.ApplicationID.String(),
		Status:        onboardingv1.ApplicationStatus(status),
	}), nil
}
