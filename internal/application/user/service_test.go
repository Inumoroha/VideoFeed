package user

import (
	"context"
	"errors"
	"testing"
	"time"

	domainuser "github.com/inumoroha/video-feed/internal/domain/user"
)

type serviceTestRepository struct {
	user             *domainuser.User
	savedUser        *domainuser.User
	findByAccountErr error
	findByIDErr      error
	saveErr          error
	updateProfileErr error
}

// Save 保存测试用户并记录传入的领域对象。
func (r *serviceTestRepository) Save(_ context.Context, user *domainuser.User) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	r.savedUser = user
	if user.ID == 0 {
		user.ID = 1
	}
	return nil
}

// FindByAccount 按账号返回测试用户或预设错误。
func (r *serviceTestRepository) FindByAccount(_ context.Context, _ string) (*domainuser.User, error) {
	if r.findByAccountErr != nil {
		return nil, r.findByAccountErr
	}
	return r.user, nil
}

// FindByID 按 ID 返回测试用户或预设错误。
func (r *serviceTestRepository) FindByID(_ context.Context, _ int64) (*domainuser.User, error) {
	if r.findByIDErr != nil {
		return nil, r.findByIDErr
	}
	return r.user, nil
}

// UpdateProfile 记录测试中的用户资料更新或返回预设错误。
func (r *serviceTestRepository) UpdateProfile(_ context.Context, user *domainuser.User) error {
	if r.updateProfileErr != nil {
		return r.updateProfileErr
	}
	r.user = user
	return nil
}

type serviceTestSigner struct {
	signErr error
	userID  int64
	role    string
}

// SignAccessToken 记录签发参数并返回固定测试令牌。
func (s *serviceTestSigner) SignAccessToken(userID int64, role string) (string, error) {
	s.userID = userID
	s.role = role
	if s.signErr != nil {
		return "", s.signErr
	}
	return "test-token", nil
}

// AccessTTL 返回测试访问令牌的有效时长。
func (serviceTestSigner) AccessTTL() time.Duration {
	return time.Hour
}

// TestRegisterReturnsProfile 验证注册成功后返回资料并回写用户 ID。
func TestRegisterReturnsProfile(t *testing.T) {
	repository := &serviceTestRepository{}
	service := NewService(repository, &serviceTestSigner{})

	profile, err := service.Register(context.Background(), "account", "password", "nickname")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if profile == nil || profile.ID != 1 || profile.Account != "account" || profile.Nickname != "nickname" {
		t.Fatalf("Register() profile = %+v, want registered profile", profile)
	}
	if repository.savedUser == nil {
		t.Fatal("Register() did not save user")
	}
}

// TestServiceRejectsMissingDependencies 验证用户服务缺少依赖时返回初始化错误而不触发 panic。
func TestServiceRejectsMissingDependencies(t *testing.T) {
	tests := []struct {
		name string
		call func(*Service) error
	}{
		{
			name: "nil service",
			call: func(service *Service) error {
				_, err := service.Register(context.Background(), "account", "password", "nickname")
				return err
			},
		},
		{
			name: "nil repository",
			call: func(service *Service) error {
				_, err := service.GetProfile(context.Background(), 1)
				return err
			},
		},
		{
			name: "nil signer",
			call: func(service *Service) error {
				_, err := service.Login(context.Background(), "account", "password")
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var service *Service
			switch test.name {
			case "nil repository":
				service = NewService(nil, &serviceTestSigner{})
			case "nil signer":
				service = NewService(&serviceTestRepository{}, nil)
			}
			if err := test.call(service); !errors.Is(err, ErrUserServiceNotInitialized) {
				t.Fatalf("service call error = %v, want %v", err, ErrUserServiceNotInitialized)
			}
		})
	}
}

// TestGetProfileRejectsNilRepositoryResult 验证仓储返回空用户时应用服务返回加载错误。
func TestGetProfileRejectsNilRepositoryResult(t *testing.T) {
	service := NewService(&serviceTestRepository{}, &serviceTestSigner{})

	_, err := service.GetProfile(context.Background(), 1)
	if !errors.Is(err, ErrLoadUserFailed) {
		t.Fatalf("GetProfile() error = %v, want %v", err, ErrLoadUserFailed)
	}
}

// TestLoginReturnsAccessToken 验证登录成功后返回令牌信息并传递用户身份。
func TestLoginReturnsAccessToken(t *testing.T) {
	user, err := domainuser.New("account", "password", "nickname")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	user.ID = 7
	signer := &serviceTestSigner{}
	service := NewService(&serviceTestRepository{user: user}, signer)

	result, err := service.Login(context.Background(), " account ", "password")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if result == nil || result.AccessToken != "test-token" || result.TokenType != "Bearer" || result.ExpiresInSeconds != 3600 {
		t.Fatalf("Login() result = %+v, want valid token result", result)
	}
	if signer.userID != 7 || signer.role != domainuser.RoleUser {
		t.Fatalf("SignAccessToken() arguments = (%d, %q), want (7, %q)", signer.userID, signer.role, domainuser.RoleUser)
	}
}

// TestGetProfileMapsErrors 验证资料查询的参数、未找到和仓储错误映射。
func TestGetProfileMapsErrors(t *testing.T) {
	tests := []struct {
		name       string
		repository *serviceTestRepository
		userID     int64
		wantErr    error
	}{
		{name: "invalid id", repository: &serviceTestRepository{}, userID: 0, wantErr: domainuser.ErrInvalidUserID},
		{name: "not found", repository: &serviceTestRepository{findByIDErr: domainuser.ErrUserNotFound}, userID: 1, wantErr: domainuser.ErrUserNotFound},
		{name: "repository failure", repository: &serviceTestRepository{findByIDErr: errors.New("database unavailable")}, userID: 1, wantErr: ErrLoadUserFailed},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := NewService(test.repository, &serviceTestSigner{})
			_, err := service.GetProfile(context.Background(), test.userID)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("GetProfile() error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

// TestGetProfileReturnsStats 验证资料查询会保留领域用户的统计字段。
func TestGetProfileReturnsStats(t *testing.T) {
	user := domainuser.RestoreUserWithStats(
		1,
		"account",
		"password-hash",
		"nickname",
		domainuser.RoleUser,
		"https://example.com/avatar.png",
		"bio",
		domainuser.StatusNormal,
		2,
		3,
		4,
	)
	service := NewService(&serviceTestRepository{user: user}, &serviceTestSigner{})

	profile, err := service.GetProfile(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("GetProfile() error = %v", err)
	}
	if profile.FollowingCount != 2 || profile.FollowerCount != 3 || profile.WorkCount != 4 {
		t.Fatalf("GetProfile() stats = (%d, %d, %d), want (2, 3, 4)", profile.FollowingCount, profile.FollowerCount, profile.WorkCount)
	}
}

// TestUpdateProfileUpdatesUser 验证资料更新会调用领域逻辑并保存更新后的用户。
func TestUpdateProfileUpdatesUser(t *testing.T) {
	user := &domainuser.User{ID: 2, Account: "account", Nickname: "old", Role: domainuser.RoleUser}
	repository := &serviceTestRepository{user: user}
	service := NewService(repository, &serviceTestSigner{})
	nickname := " new nickname "
	bio := " new bio "

	profile, err := service.UpdateProfile(context.Background(), user.ID, &nickname, nil, &bio)
	if err != nil {
		t.Fatalf("UpdateProfile() error = %v", err)
	}
	if profile == nil || profile.Nickname != "new nickname" || profile.Bio != "new bio" {
		t.Fatalf("UpdateProfile() profile = %+v, want updated profile", profile)
	}
	if repository.user.Nickname != "new nickname" || repository.user.Bio != "new bio" {
		t.Fatalf("UpdateProfile() persisted user = %+v, want updated fields", repository.user)
	}
}

// TestUpdateProfileMapsRepositoryError 验证资料更新失败时返回应用层错误。
func TestUpdateProfileMapsRepositoryError(t *testing.T) {
	user := &domainuser.User{ID: 2, Nickname: "nickname"}
	repository := &serviceTestRepository{user: user, updateProfileErr: errors.New("database unavailable")}
	service := NewService(repository, &serviceTestSigner{})
	bio := "bio"

	_, err := service.UpdateProfile(context.Background(), user.ID, nil, nil, &bio)
	if !errors.Is(err, ErrUpdateUserFailed) {
		t.Fatalf("UpdateProfile() error = %v, want %v", err, ErrUpdateUserFailed)
	}
}

var _ domainuser.Repository = (*serviceTestRepository)(nil)
var _ TokenSigner = (*serviceTestSigner)(nil)
