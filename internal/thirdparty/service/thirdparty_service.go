package service

import (
	"context"
	"errors"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/thirdparty/domain"
)

var ErrProviderUnavailable = errors.New("identity verification provider unavailable")

// IdentityVerifier is the consumer-owned port for a replaceable identity provider.
type IdentityVerifier interface {
	VerifyIdentity(context.Context, domain.Identity) (bool, error)
}

type Service struct {
	provider IdentityVerifier
}

func New(provider IdentityVerifier) *Service {
	return &Service{provider: provider}
}

func (s *Service) VerifyIdentity(ctx context.Context, identity *domain.Identity) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := domain.ValidateIdentityStructure(identity); err != nil {
		return false, err
	}
	verified, err := s.provider.VerifyIdentity(ctx, *identity)
	if err != nil {
		return false, err
	}
	return verified, nil
}
