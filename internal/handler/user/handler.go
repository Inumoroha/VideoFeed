package user

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	applicationuser "github.com/inumoroha/video-feed/internal/application/user"
	domainuser "github.com/inumoroha/video-feed/internal/domain/user"
)

// Handler 处理用户注册和登录相关的 HTTP 请求。
type Handler struct {
	service *applicationuser.Service
}

// NewHandler 创建使用指定用户应用服务的 HTTP 处理器。
func NewHandler(service *applicationuser.Service) *Handler {
	return &Handler{
		service: service,
	}
}

// Register 接收用户注册请求，校验参数并返回注册结果。
func (h *Handler) Register(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	err := h.service.Register(c.Request.Context(), req.Account, req.Password, req.Nickname)
	if err != nil {
		switch {
		case errors.Is(err, domainuser.ErrEmptyAccount),
			errors.Is(err, domainuser.ErrEmptyPassword),
			errors.Is(err, domainuser.ErrEmptyNickname):
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		case errors.Is(err, domainuser.ErrAccountAlreadyExists):
			c.JSON(http.StatusConflict, gin.H{"error": "account already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "registration successful"})
}

// Login 接收用户登录请求，验证凭据并返回访问令牌。
func (h *Handler) Login(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	var req LoginByPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	token, err := h.service.Login(c.Request.Context(), req.Account, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, domainuser.ErrEmptyAccount),
			errors.Is(err, domainuser.ErrEmptyPassword):
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		case errors.Is(err, domainuser.ErrInvalidCredentials):
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	c.JSON(http.StatusOK, tokenResponse{
		AccessToken:      token.AccessToken,
		TokenType:        token.TokenType,
		ExpiresInSeconds: token.ExpiresInSeconds,
	})
}
