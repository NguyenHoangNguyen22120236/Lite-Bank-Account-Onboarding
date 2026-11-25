package handler

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	onboardingv1 "github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/gen/onboarding/v1"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/gen/onboarding/v1/onboardingv1connect"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/thirdparty/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/thirdparty/service"
)

type Service interface {
	VerifyIdentity(context.Context, *domain.Identity) (bool, error)
}

type Handler struct {
	service Service
}

var _ onboardingv1connect.ThirdPartyServiceHandler = (*Handler)(nil)

func New(service Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) ScreenSanctions(context.Context, *connect.Request[onboardingv1.ScreenSanctionsRequest]) (*connect.Response[onboardingv1.ScreenSanctionsResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("onboarding.v1.ThirdPartyService.ScreenSanctions is not implemented"))
}

func (h *Handler) VerifyIdentity(ctx context.Context, req *connect.Request[onboardingv1.VerifyIdentityRequest]) (*connect.Response[onboardingv1.VerifyIdentityResponse], error) {
	var identity *domain.Identity
	if input := req.Msg.GetIdentity(); input != nil {
		identity = &domain.Identity{
			IDNumber: input.GetIdNumber(), FullName: input.GetFullName(),
			DateOfBirth: input.GetDateOfBirth(), IDExpiryDate: input.GetIdExpiryDate(),
			PhoneNumber: input.GetPhoneNumber(), Email: input.Email,
		}
	}
	verified, err := h.service.VerifyIdentity(ctx, identity)
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled):
			return nil, connect.NewError(connect.CodeCanceled, errors.New("request canceled"))
		case errors.Is(err, context.DeadlineExceeded):
			return nil, connect.NewError(connect.CodeDeadlineExceeded, errors.New("request deadline exceeded"))
		case errors.Is(err, domain.ErrInvalidIdentity):
			return nil, connect.NewError(connect.CodeInvalidArgument, domain.ErrInvalidIdentity)
		case errors.Is(err, service.ErrProviderUnavailable):
			return nil, connect.NewError(connect.CodeUnavailable, service.ErrProviderUnavailable)
		default:
			return nil, connect.NewError(connect.CodeInternal, errors.New("could not verify identity"))
		}
	}
	return connect.NewResponse(&onboardingv1.VerifyIdentityResponse{Verified: verified}), nil
}
