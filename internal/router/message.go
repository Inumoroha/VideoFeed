package router

import (
	"context"
	"github.com/gin-gonic/gin"
	applicationmessage "github.com/inumoroha/video-feed/internal/application/message"
	authjwt "github.com/inumoroha/video-feed/internal/auth/jwt"
	messagehandler "github.com/inumoroha/video-feed/internal/handler/message"
	"github.com/inumoroha/video-feed/internal/middleware"
)

// registerMessageRoutes 注册用户消息查询、已读及独立内部令牌保护的创建接口。
func registerMessageRoutes(g *gin.Engine, jwtManager *authjwt.Manager, handler *messagehandler.Handler, token string) {
	auth := middleware.NewJWTAuthMiddleware(jwtManager)
	g.GET("/api/messages", auth, handler.List)
	g.PATCH("/api/messages", auth, handler.MarkRead)
	g.GET("/api/message-stats/unread", auth, handler.CountUnread)
	g.POST("/internal/messages", middleware.NewInternalTokenAuth(token), handler.Create)
}

// MessageWriter 适配消息服务结果到互动与关注通知接口。
type MessageWriter struct {
	service *applicationmessage.Service
}

// NewMessageWriter 创建共享消息应用服务的通知适配器。
func NewMessageWriter(service *applicationmessage.Service) *MessageWriter {
	return &MessageWriter{service: service}
}

// CreateFromEvent 转发消息创建请求并保留创建结果和错误。
func (w *MessageWriter) CreateFromEvent(ctx context.Context, userID int64, messageType string, title string, content string, eventID string, idempotencyKey string) (any, error) {
	return w.service.CreateFromEvent(ctx, userID, messageType, title, content, eventID, idempotencyKey)
}

// CreateFromActorEvent 转发带触发用户信息的消息创建请求并保留创建结果和错误。
func (w *MessageWriter) CreateFromActorEvent(ctx context.Context, userID int64, messageType string, title string, content string, eventID string, idempotencyKey string, actorID int64, actorNickname string, actorAvatarURL string) (any, error) {
	return w.service.CreateFromActorEvent(ctx, userID, messageType, title, content, eventID, idempotencyKey, actorID, actorNickname, actorAvatarURL)
}
