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
