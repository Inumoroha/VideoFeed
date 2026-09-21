package message_test

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/inumoroha/video-feed/internal/config"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
	"time"

	applicationmessage "github.com/inumoroha/video-feed/internal/application/message"
	infrajwt "github.com/inumoroha/video-feed/internal/auth/jwt"
	domainmessage "github.com/inumoroha/video-feed/internal/domain/message"
	interfaceshttpmessage "github.com/inumoroha/video-feed/internal/handler/message"
	interfaceshttpmiddleware "github.com/inumoroha/video-feed/internal/middleware"

	"github.com/gin-gonic/gin"
)

const testInternalToken = "test-internal-token"

// messageAPIResponse 保存消息接口响应以验证字段。
type messageAPIResponse struct {
	ID        int64      `json:"id"`
	UserID    int64      `json:"user_id"`
	Type      string     `json:"type"`
	Title     string     `json:"title"`
	Content   string     `json:"content"`
	EventID   string     `json:"event_id"`
	IsRead    bool       `json:"is_read"`
	CreatedAt time.Time  `json:"created_at"`
	ReadAt    *time.Time `json:"read_at"`
}

// messageListAPIResponse 保存消息列表与分页信息以供断言。
type messageListAPIResponse struct {
	Items      []messageAPIResponse `json:"items"`
	NextCursor string               `json:"next_cursor"`
	HasMore    bool                 `json:"has_more"`
}

// unreadStatAPIResponse 保存未读数量响应。
type unreadStatAPIResponse struct {
	UnreadCount int `json:"unread_count"`
}

// markReadAPIResponse 保存实际更新数量响应。
type markReadAPIResponse struct {
	UpdatedCount int `json:"updated_count"`
}

// memoryMessageRepo 是消息测试用内存仓储，模拟事件幂等、未读计数和游标分页。
type memoryMessageRepo struct {
	mu            sync.Mutex
	nextID        int64
	messages      map[int64]*domainmessage.Message
	idsByUser     map[int64][]int64
	byUserEvent   map[memoryMessageUniqueKey]int64
	byUserRequest map[memoryMessageUniqueKey]int64
}

// memoryMessageUniqueKey 组合用户和事件或请求键以模拟唯一约束。
type memoryMessageUniqueKey struct {
	UserID int64
	Key    string
}

// newMemoryMessageRepo 初始化消息、用户索引及两类幂等索引。
func newMemoryMessageRepo() *memoryMessageRepo {
	return &memoryMessageRepo{
		nextID:        1,
		messages:      map[int64]*domainmessage.Message{},
		idsByUser:     map[int64][]int64{},
		byUserEvent:   map[memoryMessageUniqueKey]int64{},
		byUserRequest: map[memoryMessageUniqueKey]int64{},
	}
}

// Create 模拟 user_id + event_id 和 user_id + idempotency_key 两类唯一约束。
func (r *memoryMessageRepo) Create(ctx context.Context, message *domainmessage.Message, idempotencyKey string) (*domainmessage.Message, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if message.EventID != "" {
		if id, exists := r.byUserEvent[memoryMessageUniqueKey{UserID: message.UserID, Key: message.EventID}]; exists {
			return cloneMessage(r.messages[id]), false, nil
		}
	}
	if idempotencyKey != "" {
		if id, exists := r.byUserRequest[memoryMessageUniqueKey{UserID: message.UserID, Key: idempotencyKey}]; exists {
			return cloneMessage(r.messages[id]), false, nil
		}
	}

	created := cloneMessage(message)
	created.ID = r.nextID
	r.nextID++
	if created.CreatedAt.IsZero() {
		created.CreatedAt = time.Now().UTC().Add(time.Duration(created.ID) * time.Millisecond)
	}
	r.messages[created.ID] = created
	r.idsByUser[created.UserID] = append(r.idsByUser[created.UserID], created.ID)
	if created.EventID != "" {
		r.byUserEvent[memoryMessageUniqueKey{UserID: created.UserID, Key: created.EventID}] = created.ID
	}
	if idempotencyKey != "" {
		r.byUserRequest[memoryMessageUniqueKey{UserID: created.UserID, Key: idempotencyKey}] = created.ID
	}

	return cloneMessage(created), true, nil
}

// ListByUser 按 created_at DESC, id DESC 返回消息列表。
func (r *memoryMessageRepo) ListByUser(ctx context.Context, userID int64, cursor *domainmessage.Cursor, limit int) ([]*domainmessage.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	items := make([]*domainmessage.Message, 0, len(r.idsByUser[userID]))
	for _, id := range r.idsByUser[userID] {
		item := r.messages[id]
		if cursor == nil || item.CreatedAt.Before(cursor.CreatedAt) || (item.CreatedAt.Equal(cursor.CreatedAt) && item.ID < cursor.MessageID) {
			items = append(items, cloneMessage(item))
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID > items[j].ID
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	if limit > len(items) {
		limit = len(items)
	}
	return items[:limit], nil
}

// CountUnread 统计当前用户未读消息。
func (r *memoryMessageRepo) CountUnread(ctx context.Context, userID int64) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	count := 0
	for _, id := range r.idsByUser[userID] {
		if !r.messages[id].IsRead {
			count++
		}
	}
	return count, nil
}

// MarkRead 模拟批量已读，messageIDs 为空时标记当前用户全部消息。
func (r *memoryMessageRepo) MarkRead(ctx context.Context, userID int64, messageIDs []int64) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	targets := map[int64]struct{}{}
	for _, id := range messageIDs {
		targets[id] = struct{}{}
	}

	updated := 0
	now := time.Now().UTC()
	for _, id := range r.idsByUser[userID] {
		message := r.messages[id]
		if len(targets) > 0 {
			if _, exists := targets[id]; !exists {
				continue
			}
		}
		if message.IsRead {
			continue
		}
		message.IsRead = true
		message.ReadAt = &now
		updated++
	}
	return updated, nil
}

// cloneMessage 复制消息及可空已读时间，隔离测试中的共享引用。
func cloneMessage(message *domainmessage.Message) *domainmessage.Message {
	cloned := *message
	if message.ReadAt != nil {
		readAt := *message.ReadAt
		cloned.ReadAt = &readAt
	}
	return &cloned
}

// TestMessageAPIFlow 验证创建、重放、分页、用户隔离、未读统计及批量和全部已读。
func TestMessageAPIFlow(t *testing.T) {
	router, jwtManager := newMessageRouter(t)
	token := signTestToken(t, jwtManager, 42)
	otherToken := signTestToken(t, jwtManager, 77)

	firstResponse := performInternalMessageRequest(
		router,
		http.MethodPost,
		"/internal/messages",
		`{"user_id":42,"type":"like","title":"收到点赞","content":"你的作品收到了点赞","event_id":"evt-1"}`,
		testInternalToken,
		"msg-1",
	)
	requireStatus(t, firstResponse, http.StatusCreated)

	var first messageAPIResponse
	decodeJSON(t, firstResponse, &first)
	if first.ID == 0 || first.UserID != 42 || first.Type != domainmessage.TypeLike || first.IsRead {
		t.Fatalf("unexpected created message: %+v", first)
	}

	replayResponse := performInternalMessageRequest(
		router,
		http.MethodPost,
		"/internal/messages",
		`{"user_id":42,"type":"like","title":"changed","content":"changed","event_id":"evt-1"}`,
		testInternalToken,
		"msg-1",
	)
	requireStatus(t, replayResponse, http.StatusOK)

	var replay messageAPIResponse
	decodeJSON(t, replayResponse, &replay)
	if replay.ID != first.ID || replay.Title != first.Title {
		t.Fatalf("expected idempotent replay, got %+v", replay)
	}

	requireStatus(t, performInternalMessageRequest(router, http.MethodPost, "/internal/messages", `{"user_id":42,"type":"comment","title":"收到评论","content":"有人评论了你的作品","event_id":"evt-2"}`, testInternalToken, "msg-2"), http.StatusCreated)
	requireStatus(t, performInternalMessageRequest(router, http.MethodPost, "/internal/messages", `{"user_id":42,"type":"follow","title":"新增关注","content":"有人关注了你","event_id":"evt-3"}`, testInternalToken, "msg-3"), http.StatusCreated)
	requireStatus(t, performInternalMessageRequest(router, http.MethodPost, "/internal/messages", `{"user_id":77,"type":"system","title":"系统通知","content":"欢迎回来","event_id":"evt-4"}`, testInternalToken, "msg-4"), http.StatusCreated)

	firstPageResponse := performJSONRequest(router, http.MethodGet, "/api/messages?limit=2", "", token)
	requireStatus(t, firstPageResponse, http.StatusOK)

	var firstPage messageListAPIResponse
	decodeJSON(t, firstPageResponse, &firstPage)
	if len(firstPage.Items) != 2 || !firstPage.HasMore || firstPage.NextCursor == "" {
		t.Fatalf("unexpected first page: %+v", firstPage)
	}
	if firstPage.Items[0].ID <= firstPage.Items[1].ID {
		t.Fatalf("expected newest message first, got %+v", firstPage.Items)
	}

	secondPageResponse := performJSONRequest(router, http.MethodGet, "/api/messages?limit=2&cursor="+firstPage.NextCursor, "", token)
	requireStatus(t, secondPageResponse, http.StatusOK)

	var secondPage messageListAPIResponse
	decodeJSON(t, secondPageResponse, &secondPage)
	if len(secondPage.Items) != 1 || secondPage.HasMore {
		t.Fatalf("unexpected second page: %+v", secondPage)
	}

	otherListResponse := performJSONRequest(router, http.MethodGet, "/api/messages?limit=10", "", otherToken)
	requireStatus(t, otherListResponse, http.StatusOK)

	var otherList messageListAPIResponse
	decodeJSON(t, otherListResponse, &otherList)
	if len(otherList.Items) != 1 || otherList.Items[0].UserID != 77 {
		t.Fatalf("expected isolated user messages, got %+v", otherList)
	}

	statResponse := performJSONRequest(router, http.MethodGet, "/api/message-stats/unread", "", token)
	requireStatus(t, statResponse, http.StatusOK)

	var stat unreadStatAPIResponse
	decodeJSON(t, statResponse, &stat)
	if stat.UnreadCount != 3 {
		t.Fatalf("expected 3 unread messages, got %+v", stat)
	}

	markResponse := performJSONRequest(router, http.MethodPatch, "/api/messages", `{"message_ids":[1,3,3]}`, token)
	requireStatus(t, markResponse, http.StatusOK)

	var mark markReadAPIResponse
	decodeJSON(t, markResponse, &mark)
	if mark.UpdatedCount != 2 {
		t.Fatalf("expected 2 messages marked read, got %+v", mark)
	}

	markAllResponse := performJSONRequest(router, http.MethodPatch, "/api/messages", `{}`, token)
	requireStatus(t, markAllResponse, http.StatusOK)

	var markAll markReadAPIResponse
	decodeJSON(t, markAllResponse, &markAll)
	if markAll.UpdatedCount != 1 {
		t.Fatalf("expected remaining message marked read, got %+v", markAll)
	}

	emptyStatResponse := performJSONRequest(router, http.MethodGet, "/api/message-stats/unread", "", token)
	requireStatus(t, emptyStatResponse, http.StatusOK)
	decodeJSON(t, emptyStatResponse, &stat)
	if stat.UnreadCount != 0 {
		t.Fatalf("expected no unread messages, got %+v", stat)
	}
}

// TestMessageAPIValidation 验证消息接口认证和参数错误响应。
func TestMessageAPIValidation(t *testing.T) {
	router, jwtManager := newMessageRouter(t)
	token := signTestToken(t, jwtManager, 42)

	requireStatus(t, performJSONRequest(router, http.MethodGet, "/api/messages", "", ""), http.StatusUnauthorized)
	requireStatus(t, performJSONRequest(router, http.MethodGet, "/api/message-stats/unread", "", ""), http.StatusUnauthorized)
	requireStatus(t, performJSONRequest(router, http.MethodPatch, "/api/messages", `{}`, ""), http.StatusUnauthorized)

	requireStatus(t, performJSONRequest(router, http.MethodGet, "/api/messages?limit=0", "", token), http.StatusBadRequest)
	requireStatus(t, performJSONRequest(router, http.MethodGet, "/api/messages?cursor=bad", "", token), http.StatusBadRequest)
	requireStatus(t, performJSONRequest(router, http.MethodPatch, "/api/messages", `{"message_ids":[0]}`, token), http.StatusBadRequest)
	requireStatus(t, performInternalMessageRequest(router, http.MethodPost, "/internal/messages", `{"user_id":42,"type":"bad","title":"x","content":"x"}`, testInternalToken, ""), http.StatusBadRequest)
	requireStatus(t, performInternalMessageRequest(router, http.MethodPost, "/internal/messages", `{"user_id":0,"type":"system","title":"x","content":"x"}`, testInternalToken, ""), http.StatusBadRequest)
	requireStatus(t, performInternalMessageRequest(router, http.MethodPost, "/internal/messages", `{"user_id":42,"type":"system","title":"","content":"x"}`, testInternalToken, ""), http.StatusBadRequest)
	requireStatus(t, performInternalMessageRequest(router, http.MethodPost, "/internal/messages", `{"user_id":42,"type":"system","title":"x","content":"x"}`, "", ""), http.StatusUnauthorized)
	requireStatus(t, performInternalMessageRequest(router, http.MethodPost, "/internal/messages", `{"user_id":42,"type":"system","title":"x","content":"x"}`, "wrong-token", ""), http.StatusUnauthorized)
}

// newMessageRouter 组装内存消息仓储、访问令牌认证和内部令牌认证路由。
func newMessageRouter(t *testing.T) (*gin.Engine, *infrajwt.Manager) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	repo := newMemoryMessageRepo()
	service := applicationmessage.New(repo)
	handler := interfaceshttpmessage.New(service)
	jwtManager, err := infrajwt.NewManager(config.JWTConfig{Secret: "test-message-secret-at-least-32-bytes", AccessTTL: 15 * time.Minute, RefreshTTL: 24 * time.Hour})
	if err != nil {
		t.Fatalf("new jwt manager: %v", err)
	}

	router := gin.New()
	authMiddleware := interfaceshttpmiddleware.NewJWTAuthMiddleware(jwtManager)
	api := router.Group("/api", authMiddleware)
	api.GET("/messages", handler.List)
	api.PATCH("/messages", handler.MarkRead)
	api.GET("/message-stats/unread", handler.CountUnread)
	router.POST("/internal/messages", interfaceshttpmiddleware.NewInternalTokenAuth(testInternalToken), handler.Create)

	return router, jwtManager
}

// performInternalMessageRequest 发送携带内部令牌和幂等键的测试请求。
func performInternalMessageRequest(router *gin.Engine, method, path, body, internalToken, idempotencyKey string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if internalToken != "" {
		req.Header.Set("X-Internal-Token", internalToken)
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

// signTestToken 签发指定用户的测试访问令牌。
func signTestToken(t *testing.T, m *infrajwt.Manager, id int64) string {
	t.Helper()
	token, err := m.SignAccessToken(id, "user")
	if err != nil {
		t.Fatal(err)
	}
	return token
}

// performJSONRequest 发送携带可选用户令牌的请求并记录响应。
func performJSONRequest(router *gin.Engine, method, path, body, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

// requireStatus 断言响应状态并在失败时输出响应正文。
func requireStatus(t *testing.T, r *httptest.ResponseRecorder, status int) {
	t.Helper()
	if r.Code != status {
		t.Fatalf("status=%d want=%d body=%s", r.Code, status, r.Body.String())
	}
}

// decodeJSON 将响应正文解码到断言对象，解码失败立即终止测试。
func decodeJSON(t *testing.T, r *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(r.Body.Bytes(), target); err != nil {
		t.Fatal(err)
	}
}
