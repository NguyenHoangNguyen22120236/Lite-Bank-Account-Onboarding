package handler

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	onboardingv1 "github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/gen/onboarding/v1"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/service"
)

type StartApplicationService interface {
	StartApplication(context.Context, string) (domain.Application, error)
}

func (h *Handler) StartApplication(ctx context.Context, req *connect.Request[onboardingv1.StartApplicationRequest]) (*connect.Response[onboardingv1.StartApplicationResponse], error) {
	app, err := h.startApplication.StartApplication(ctx, req.Msg.GetRequestId())
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
