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
func (s *Service) Register(ctx context.Context, account, password, nickname string) (*Profile, error) {
	if !s.ready() {
		return nil, ErrUserServiceNotInitialized
	}

	user, err := domainuser.New(account, password, nickname)
	if err != nil {
		return nil, err
	}

	err = s.repo.Save(ctx, user)
	if err != nil {
		if errors.Is(err, domainuser.ErrAccountAlreadyExists) {
			return nil, domainuser.ErrAccountAlreadyExists
		}
		return nil, ErrSaveUserFailed
	}
	return profileFromUser(user), nil
}

// Login 验证用户凭据并签发访问令牌，返回令牌类型和有效时长。
func (s *Service) Login(ctx context.Context, account, password string) (*LoginResult, error) {
	if !s.ready() {
		return nil, ErrUserServiceNotInitialized
	}

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
	if user == nil {
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

// GetProfile 根据用户 ID 查询用户资料，并将仓储错误转换为应用层错误。
func (s *Service) GetProfile(ctx context.Context, userID int64) (*Profile, error) {
	if !s.ready() {
		return nil, ErrUserServiceNotInitialized
	}
	if userID <= 0 {
		return nil, domainuser.ErrInvalidUserID
	}

	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, domainuser.ErrUserNotFound) {
			return nil, domainuser.ErrUserNotFound
		}
		return nil, ErrLoadUserFailed
	}
	if user == nil {
		return nil, ErrLoadUserFailed
	}

	return profileFromUser(user), nil
}

// GetPublicProfile 根据用户 ID 查询可公开展示的用户资料，并复用当前用户资料的错误处理。
func (s *Service) GetPublicProfile(ctx context.Context, userID int64) (*Profile, error) {
	return s.GetProfile(ctx, userID)
}

// UpdateProfile 查询用户、更新资料并保存，返回更新后的用户资料。
func (s *Service) UpdateProfile(ctx context.Context, userID int64, nickname, avatarURL, bio *string) (*Profile, error) {
	if !s.ready() {
		return nil, ErrUserServiceNotInitialized
	}
	if userID <= 0 {
		return nil, domainuser.ErrInvalidUserID
	}

	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, domainuser.ErrUserNotFound) {
			return nil, domainuser.ErrUserNotFound
		}
		return nil, ErrLoadUserFailed
	}
	if user == nil {
		return nil, ErrLoadUserFailed
	}
	if err := user.UpdateProfile(nickname, avatarURL, bio); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateProfile(ctx, user); err != nil {
		if errors.Is(err, domainuser.ErrUserNotFound) {
			return nil, domainuser.ErrUserNotFound
		}
		return nil, ErrUpdateUserFailed
	}

	return profileFromUser(user), nil
}

// profileFromUser 将领域用户转换为应用层用户资料。
func profileFromUser(user *domainuser.User) *Profile {
	return &Profile{
		ID:             user.ID,
		Account:        user.Account,
		Nickname:       user.Nickname,
		AvatarURL:      user.AvatarURL,
		Bio:            user.Bio,
		Status:         user.Status,
		Role:           user.Role,
		FollowingCount: user.FollowingCount,
		FollowerCount:  user.FollowerCount,
		WorkCount:      user.WorkCount,
	}
}

// ready 检查用户服务及其仓储、令牌签发器是否已初始化。
func (s *Service) ready() bool {
	return s != nil && s.repo != nil && s.signer != nil
}
