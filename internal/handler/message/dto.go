package message

import "time"

// createMessageRequest 接收内部消息创建字段。
type createMessageRequest struct {
	UserID  int64  `json:"user_id"`
	Type    string `json:"type"`
	Title   string `json:"title"`
	Content string `json:"content"`
	EventID string `json:"event_id"`
}

// markReadRequest 接收待标记的消息标识，空列表表示全部。
type markReadRequest struct {
	MessageIDs []int64 `json:"message_ids"`
}

// messageResponse 保存对外消息字段及可选的事件和已读时间。
type messageResponse struct {
	ID        int64      `json:"id"`
	UserID    int64      `json:"user_id"`
	Type      string     `json:"type"`
	Title     string     `json:"title"`
	Content   string     `json:"content"`
	EventID   string     `json:"event_id,omitempty"`
	IsRead    bool       `json:"is_read"`
	CreatedAt time.Time  `json:"created_at"`
	ReadAt    *time.Time `json:"read_at,omitempty"`
}

// messageListResponse 保存消息列表和游标分页响应。
type messageListResponse struct {
	Items      []messageResponse `json:"items"`
	NextCursor string            `json:"next_cursor"`
	HasMore    bool              `json:"has_more"`
}

// unreadStatResponse 保存未读数量响应。
type unreadStatResponse struct {
	UnreadCount int `json:"unread_count"`
}

// markReadResponse 保存已读更新数量响应。
type markReadResponse struct {
	UpdatedCount int `json:"updated_count"`
}
