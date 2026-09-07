package user

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	applicationuser "github.com/inumoroha/video-feed/internal/application/user"
	domainuser "github.com/inumoroha/video-feed/internal/domain/user"
	"github.com/inumoroha/video-feed/internal/middleware"
)

// testRepository 提供用户 Handler 测试所需的用户仓储实现。
type testRepository struct {
	user *domainuser.User
}

// Save 保存用户测试仓储接收的用户。
func (r *testRepository) Save(_ context.Context, user *domainuser.User) error {
	r.user = user
	return nil
}

// FindByAccount 按账号返回预设用户或未找到错误。
func (r *testRepository) FindByAccount(_ context.Context, account string) (*domainuser.User, error) {
	if r.user == nil || r.user.Account != account {
		return nil, domainuser.ErrUserNotFound
	}
	return r.user, nil
}

// FindByID 按 ID 返回预设用户或未找到错误。
func (r *testRepository) FindByID(_ context.Context, id int64) (*domainuser.User, error) {
	if r.user == nil || r.user.ID != id {
		return nil, domainuser.ErrUserNotFound
	}
	return r.user, nil
}

// UpdateProfile 保存测试中的用户资料更新。
func (r *testRepository) UpdateProfile(_ context.Context, user *domainuser.User) error {
	r.user = user
	return nil
}

// testTokenSigner 提供用户 Handler 测试所需的令牌签发能力。
type testTokenSigner struct{}

// SignAccessToken 返回固定测试令牌。
func (testTokenSigner) SignAccessToken(_ int64, _ string) (string, error) {
	return "test-token", nil
}

// AccessTTL 返回测试访问令牌的有效时长。
func (testTokenSigner) AccessTTL() time.Duration {
	return time.Hour
}

// TestRegisterReturnsBadRequestForWhitespaceInput 验证注册接口将领域输入校验错误映射为 400。
func TestRegisterReturnsBadRequestForWhitespaceInput(t *testing.T) {
	service := applicationuser.NewService(&testRepository{}, testTokenSigner{})
	handler := NewHandler(service)
	recorder := httptest.NewRecorder()
	ginContext := newTestContext(http.MethodPost, "/users/register", `{"account":"   ","password":"password","nickname":"nickname"}`, recorder)

	handler.Register(ginContext)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("Register() status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

// TestLoginReturnsBadRequestForWhitespacePassword 验证登录接口将空白密码错误映射为 400。
func TestLoginReturnsBadRequestForWhitespacePassword(t *testing.T) {
	user, err := domainuser.New("account", "password", "nickname")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	service := applicationuser.NewService(&testRepository{user: user}, testTokenSigner{})
	handler := NewHandler(service)
	recorder := httptest.NewRecorder()
	ginContext := newTestContext(http.MethodPost, "/users/login", `{"account":"account","password":"   "}`, recorder)

	handler.Login(ginContext)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("Login() status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

// TestRegisterWithNilServiceReturnsInternalServerError 验证缺少应用服务时接口返回 500 而不会 panic。
func TestRegisterWithNilServiceReturnsInternalServerError(t *testing.T) {
	handler := NewHandler(nil)
	recorder := httptest.NewRecorder()
	ginContext := newTestContext(http.MethodPost, "/users/register", `{}`, recorder)

	handler.Register(ginContext)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("Register() status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}

// TestMeReturnsUnauthorizedWithoutUserID 验证当前用户资料接口缺少认证用户 ID 时返回 401。
func TestMeReturnsUnauthorizedWithoutUserID(t *testing.T) {
	service := applicationuser.NewService(&testRepository{}, testTokenSigner{})
	recorder := httptest.NewRecorder()
	ginContext := newTestContext(http.MethodGet, "/users/me", "", recorder)

	NewHandler(service).Me(ginContext)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("Me() status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

// TestMeReturnsProfile 验证当前用户资料接口返回认证用户的资料。
func TestMeReturnsProfile(t *testing.T) {
	user := &domainuser.User{
		ID:             1,
		Account:        "account",
		Nickname:       "nickname",
		Role:           domainuser.RoleUser,
		Bio:            "bio",
		AvatarURL:      "https://example.com/avatar.png",
		Status:         domainuser.StatusNormal,
		FollowingCount: 2,
		FollowerCount:  3,
		WorkCount:      4,
	}
	service := applicationuser.NewService(&testRepository{user: user}, testTokenSigner{})
	recorder := httptest.NewRecorder()
	ginContext := newTestContext(http.MethodGet, "/users/me", "", recorder)
	ginContext.Set(middleware.ContextUserIDKey, int64(1))

	NewHandler(service).Me(ginContext)

	if recorder.Code != http.StatusOK {
		t.Fatalf("Me() status = %d, want %d", recorder.Code, http.StatusOK)
	}
	response := recorder.Body.String()
	for _, field := range []string{`"id":1`, `"account":"account"`, `"nickname":"nickname"`, `"bio":"bio"`, `"status":1`, `"following_count":2`, `"follower_count":3`, `"work_count":4`} {
		if !strings.Contains(response, field) {
			t.Fatalf("Me() response = %s, missing %s", response, field)
		}
	}
}

// TestMeWithNilServiceReturnsInternalServerError 验证当前用户资料接口缺少应用服务时不会 panic。
func TestMeWithNilServiceReturnsInternalServerError(t *testing.T) {
	recorder := httptest.NewRecorder()
	ginContext := newTestContext(http.MethodGet, "/users/me", "", recorder)
	ginContext.Set(middleware.ContextUserIDKey, int64(1))

	NewHandler(nil).Me(ginContext)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("Me() status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}

// TestGetReturnsPublicProfile 验证公开用户资料接口只返回公开字段和统计数据。
func TestGetReturnsPublicProfile(t *testing.T) {
	user := &domainuser.User{
		ID:             1,
		Account:        "private-account",
		Nickname:       "nickname",
		Role:           domainuser.RoleAdmin,
		Bio:            "bio",
		AvatarURL:      "https://example.com/avatar.png",
		Status:         domainuser.StatusNormal,
		FollowingCount: 2,
		FollowerCount:  3,
		WorkCount:      4,
	}
	service := applicationuser.NewService(&testRepository{user: user}, testTokenSigner{})
	recorder := httptest.NewRecorder()
	ginContext := newTestContext(http.MethodGet, "/users/1", "", recorder)
	ginContext.Params = gin.Params{{Key: "userId", Value: "1"}}

	NewHandler(service).Get(ginContext)

	if recorder.Code != http.StatusOK {
		t.Fatalf("Get() status = %d, want %d", recorder.Code, http.StatusOK)
	}
	response := recorder.Body.String()
	for _, field := range []string{`"id":1`, `"nickname":"nickname"`, `"bio":"bio"`, `"following_count":2`, `"follower_count":3`, `"work_count":4`} {
		if !strings.Contains(response, field) {
			t.Fatalf("Get() response = %s, missing %s", response, field)
		}
	}
	for _, field := range []string{`"account"`, `"role"`, `"status"`} {
		if strings.Contains(response, field) {
			t.Fatalf("Get() response = %s, must not contain %s", response, field)
		}
	}
}

// TestGetWithNilServiceReturnsInternalServerError 验证公开用户资料接口缺少应用服务时不会 panic。
func TestGetWithNilServiceReturnsInternalServerError(t *testing.T) {
	recorder := httptest.NewRecorder()
	ginContext := newTestContext(http.MethodGet, "/users/1", "", recorder)
	ginContext.Params = gin.Params{{Key: "userId", Value: "1"}}

	NewHandler(nil).Get(ginContext)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("Get() status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}

// TestGetRejectsInvalidUserID 验证公开用户资料接口拒绝非正整数用户 ID。
func TestGetRejectsInvalidUserID(t *testing.T) {
	service := applicationuser.NewService(&testRepository{}, testTokenSigner{})
	recorder := httptest.NewRecorder()
	ginContext := newTestContext(http.MethodGet, "/users/not-an-id", "", recorder)
	ginContext.Params = gin.Params{{Key: "userId", Value: "not-an-id"}}

	NewHandler(service).Get(ginContext)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("Get() status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

// TestUpdateMeReturnsBadRequestForEmptyPatch 验证当前用户资料接口拒绝没有更新字段的请求。
func TestUpdateMeReturnsBadRequestForEmptyPatch(t *testing.T) {
	user := &domainuser.User{ID: 1, Nickname: "nickname"}
	service := applicationuser.NewService(&testRepository{user: user}, testTokenSigner{})
	recorder := httptest.NewRecorder()
	ginContext := newTestContext(http.MethodPatch, "/users/me", `{}`, recorder)
	ginContext.Set(middleware.ContextUserIDKey, int64(1))

	NewHandler(service).UpdateMe(ginContext)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("UpdateMe() status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

// TestUpdateMeWithNilServiceReturnsInternalServerError 验证资料更新接口缺少应用服务时不会 panic。
func TestUpdateMeWithNilServiceReturnsInternalServerError(t *testing.T) {
	recorder := httptest.NewRecorder()
	ginContext := newTestContext(http.MethodPatch, "/users/me", `{}`, recorder)
	ginContext.Set(middleware.ContextUserIDKey, int64(1))

	NewHandler(nil).UpdateMe(ginContext)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("UpdateMe() status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}

// TestLogoutReturnsNoContent 验证退出登录接口返回无内容响应。
func TestLogoutReturnsNoContent(t *testing.T) {
	recorder := httptest.NewRecorder()
	ginContext := newTestContext(http.MethodPost, "/auth/logout", "", recorder)

	NewHandler(nil).Logout(ginContext)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("Logout() status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}

// newTestContext 创建携带 JSON 请求体的 Gin 测试上下文。
func newTestContext(method, target, body string, recorder *httptest.ResponseRecorder) *gin.Context {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	ginContext, _ := gin.CreateTestContext(recorder)
	ginContext.Request = request
	return ginContext
}

var _ domainuser.Repository = (*testRepository)(nil)
var _ applicationuser.TokenSigner = testTokenSigner{}
