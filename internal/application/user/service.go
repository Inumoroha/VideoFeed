package user

import (
	"context"
	"errors"
	"strings"
	"time"

	domainuser "github.com/inumoroha/video-feed/internal/domain/user"
)

// TokenSigner 定义用户应用服务签发访问令牌所需的能力。
type TokenSigner interface {
	// SignAccessToken 为指定用户签发访问令牌。
	SignAccessToken(userID int64, role string) (string, error)
	// AccessTTL 返回访问令牌的有效时长。
	AccessTTL() time.Duration
}

// Service 编排用户注册、登录等应用层业务流程。
type Service struct {
	repo   domainuser.Repository
	signer TokenSigner
}

// NewService 创建使用指定用户仓储和令牌签发器的用户应用服务。
func NewService(repo domainuser.Repository, signer TokenSigner) *Service {
	return &Service{
		repo:   repo,
		signer: signer,
	}
}

// Register 创建用户并将其保存到用户仓储中。
func (s *Service) Register(ctx context.Context, account, password, nickname string) error {
	user, err := domainuser.New(account, password, nickname)
	if err != nil {
		return err
	}

	err = s.repo.Save(ctx, user)
	if err != nil {
		if errors.Is(err, domainuser.ErrAccountAlreadyExists) {
			return domainuser.ErrAccountAlreadyExists
		}
		return ErrSaveUserFailed
	}
	return nil
}

// Login 验证用户凭据并签发访问令牌，返回令牌类型和有效时长。
func (s *Service) Login(ctx context.Context, account, password string) (*LoginResult, error) {
	account = strings.TrimSpace(account)
	if account == "" {
		return nil, domainuser.ErrEmptyAccount
	}

	user, err := s.repo.FindByAccount(ctx, account)
	if err != nil {
		if errors.Is(err, domainuser.ErrUserNotFound) {
			return nil, domainuser.ErrInvalidCredentials
		}
		return nil, ErrLoadUserFailed
	}
	if err := user.Authenticate(password); err != nil {
		return nil, err
	}

	accessToken, err := s.signer.SignAccessToken(user.ID, user.Role)
	if err != nil {
		return nil, ErrSignAccessTokenFailed
	}

	return &LoginResult{
		AccessToken:      accessToken,
		TokenType:        "Bearer",
		ExpiresInSeconds: int64(s.signer.AccessTTL().Seconds()),
	}, nil
}
