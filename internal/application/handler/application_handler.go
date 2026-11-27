package handler

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	onboardingv1 "github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/gen/onboarding/v1"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/gen/onboarding/v1/onboardingv1connect"
)

type Handler struct {
	startApplication StartApplicationService
	acceptTerms      AcceptTermsService
	submitIdentity   SubmitIdentityService
}

var _ onboardingv1connect.ApplicationServiceHandler = (*Handler)(nil)

func New(startApplication StartApplicationService, acceptTerms AcceptTermsService, submitIdentity SubmitIdentityService) *Handler {
	return &Handler{startApplication: startApplication, acceptTerms: acceptTerms, submitIdentity: submitIdentity}
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
