package user

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	applicationuser "github.com/inumoroha/video-feed/internal/application/user"
	domainuser "github.com/inumoroha/video-feed/internal/domain/user"
	"github.com/inumoroha/video-feed/internal/middleware"
)

// Handler 处理用户注册、登录、退出登录和用户资料相关的 HTTP 请求。
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

	profile, err := h.service.Register(c.Request.Context(), req.Account, req.Password, req.Nickname)
	if err != nil {
		switch {
		case isBadRequestError(err):
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		case errors.Is(err, domainuser.ErrAccountAlreadyExists):
			c.JSON(http.StatusConflict, gin.H{"error": "account already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	if profile == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	c.JSON(http.StatusCreated, profileResponse(profile))
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
		case isBadRequestError(err):
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		case errors.Is(err, domainuser.ErrInvalidCredentials):
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	if token == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	c.JSON(http.StatusOK, tokenResponse{
		AccessToken:      token.AccessToken,
		TokenType:        token.TokenType,
		ExpiresInSeconds: token.ExpiresInSeconds,
	})
}

// Logout 返回无内容响应；当前使用无状态 JWT，客户端应删除本地令牌完成退出登录。
func (h *Handler) Logout(c *gin.Context) {
	c.AbortWithStatus(http.StatusNoContent)
}

// Me 根据认证中间件写入的用户 ID 返回当前用户资料。
func (h *Handler) Me(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}

	profile, err := h.service.GetProfile(c.Request.Context(), userID)
	if err != nil {
		writeProfileError(c, err)
		return
	}

	if profile == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	c.JSON(http.StatusOK, profileResponse(profile))
}

// Get 读取公开用户资料，用于访问他人主页。
func (h *Handler) Get(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	userID, err := parsePositiveUserID(c.Param("userId"))
	if err != nil {
		writeProfileError(c, err)
		return
	}

	profile, err := h.service.GetPublicProfile(c.Request.Context(), userID)
	if err != nil {
		writeProfileError(c, err)
		return
	}

	if profile == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	c.JSON(http.StatusOK, publicProfileResponse(profile))
}

// UpdateMe 解析当前用户的资料更新请求并返回更新后的资料。
func (h *Handler) UpdateMe(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}

	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	profile, err := h.service.UpdateProfile(c.Request.Context(), userID, req.Nickname, req.AvatarURL, req.Bio)
	if err != nil {
		writeProfileError(c, err)
		return
	}

	if profile == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	c.JSON(http.StatusOK, profileResponse(profile))
}

// publicProfileResponse 将应用层 Profile 转成公开 JSON 结构。
func publicProfileResponse(profile *applicationuser.Profile) publicUserProfileResponse {
	return publicUserProfileResponse{
		ID:             profile.ID,
		Nickname:       profile.Nickname,
		AvatarURL:      profile.AvatarURL,
		Bio:            profile.Bio,
		FollowingCount: profile.FollowingCount,
		FollowerCount:  profile.FollowerCount,
		WorkCount:      profile.WorkCount,
	}
}

// profileResponse 将应用层用户资料转换为 HTTP 响应结构。
func profileResponse(profile *applicationuser.Profile) userProfileResponse {
	return userProfileResponse{
		ID:             profile.ID,
		Account:        profile.Account,
		Nickname:       profile.Nickname,
		AvatarURL:      profile.AvatarURL,
		Bio:            profile.Bio,
		Status:         profile.Status,
		Role:           profile.Role,
		FollowingCount: profile.FollowingCount,
		FollowerCount:  profile.FollowerCount,
		WorkCount:      profile.WorkCount,
	}
}

// parsePositiveUserID 将字符串解析为正整数用户 ID。
func parsePositiveUserID(raw string) (int64, error) {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || value <= 0 {
		return 0, domainuser.ErrInvalidUserID
	}
	return value, nil
}

// userIDFromContext 读取认证中间件写入的正数用户 ID。
func userIDFromContext(c *gin.Context) (int64, bool) {
	value, exists := c.Get(middleware.ContextUserIDKey)
	if !exists {
		return 0, false
	}
	userID, ok := value.(int64)
	return userID, ok && userID > 0
}

// writeProfileError 将用户资料相关错误转换为统一的 HTTP 响应。
func writeProfileError(c *gin.Context, err error) {
	if isBadRequestError(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if errors.Is(err, domainuser.ErrUserNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}

// isBadRequestError 检查错误是否为客户端错误。
func isBadRequestError(err error) bool {
	return errors.Is(err, domainuser.ErrEmptyAccount) ||
		errors.Is(err, domainuser.ErrEmptyPassword) ||
		errors.Is(err, domainuser.ErrEmptyNickname) ||
		errors.Is(err, domainuser.ErrInvalidUserID) ||
		errors.Is(err, domainuser.ErrEmptyProfileUpdate)
}
