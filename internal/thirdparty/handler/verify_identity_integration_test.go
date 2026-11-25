package handler_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	onboardingv1 "github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/gen/onboarding/v1"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/gen/onboarding/v1/onboardingv1connect"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/thirdparty/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/thirdparty/handler"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/thirdparty/mock"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/thirdparty/service"
)

type verifierFunc func(context.Context, domain.Identity) (bool, error)

func (f verifierFunc) VerifyIdentity(ctx context.Context, i domain.Identity) (bool, error) {
	return f(ctx, i)
}

func TestVerifyIdentityAPI(t *testing.T) {
	mockProvider := mock.NewIdentityVerifier()
	for _, tt := range []struct {
		name        string
		id          string
		providerErr error
		verified    bool
		code        connect.Code
	}{
		{"verified", "000000000001", nil, true, 0},
		{"unverified", "000000000999", nil, false, 0},
		{"invalid input", "bad", nil, false, connect.CodeInvalidArgument},
		{"canceled", "000000000001", fmt.Errorf("private 000000000001: %w", context.Canceled), false, connect.CodeCanceled},
		{"deadline", "000000000001", fmt.Errorf("private 000000000001: %w", context.DeadlineExceeded), false, connect.CodeDeadlineExceeded},
		{"unavailable", "000000000001", fmt.Errorf("private 000000000001: %w", service.ErrProviderUnavailable), false, connect.CodeUnavailable},
		{"unexpected error overrides true", "000000000001", errors.New("private 000000000001"), false, connect.CodeInternal},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Inject failures at the provider boundary; exercise the real service and handler.
			provider := verifierFunc(func(ctx context.Context, i domain.Identity) (bool, error) {
				if tt.providerErr != nil {
					return true, tt.providerErr
				}
				return mockProvider.VerifyIdentity(ctx, i)
			})
			mux := http.NewServeMux()
			path, rpc := onboardingv1connect.NewThirdPartyServiceHandler(handler.New(service.New(provider)))
			mux.Handle(path, rpc)
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.ProtoMajor != 2 || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc") {
					t.Error("expected gRPC over HTTP/2")
				}
				mux.ServeHTTP(w, r)
			}))
			server.Config.Protocols = new(http.Protocols)
			server.Config.Protocols.SetUnencryptedHTTP2(true)
			server.Start()
			defer server.Close()
			protocols := new(http.Protocols)
			protocols.SetUnencryptedHTTP2(true)
			transport := &http.Transport{Protocols: protocols}
			defer transport.CloseIdleConnections()
			client := onboardingv1connect.NewThirdPartyServiceClient(&http.Client{Transport: transport}, server.URL, connect.WithGRPC())
			input := &onboardingv1.IdentityInfo{IdNumber: tt.id, FullName: "Mock Citizen", DateOfBirth: "1990-01-01", IdExpiryDate: "2040-01-01", PhoneNumber: "0900000000"}
			response, err := client.VerifyIdentity(context.Background(), connect.NewRequest(&onboardingv1.VerifyIdentityRequest{Identity: input}))
			if tt.code == 0 {
				if err != nil || response == nil || response.Msg.Verified != tt.verified {
					t.Fatalf("response=%v, error=%v", response, err)
				}
			} else {
				if response != nil || connect.CodeOf(err) != tt.code {
					t.Fatalf("response=%v, error=%v; want %v", response, err, tt.code)
				}
				for _, secret := range []string{"private", "000000000001", "Mock Citizen"} {
					if strings.Contains(err.Error(), secret) {
						t.Fatalf("error exposes %q: %v", secret, err)
					}
				}
			}
			if tt.name == "verified" {
				_, err := client.ScreenSanctions(context.Background(), connect.NewRequest(&onboardingv1.ScreenSanctionsRequest{Identity: input}))
				if connect.CodeOf(err) != connect.CodeUnimplemented {
					t.Fatalf("ScreenSanctions error=%v", err)
				}
			}
		})
	}
}
