package user

// RegisterRequest 表示用户注册请求。
type RegisterRequest struct {
	Account  string `json:"account" binding:"required"`
	Password string `json:"password" binding:"required"`
	Nickname string `json:"nickname" binding:"required"`
}

// LoginByPasswordRequest 表示使用密码登录的请求。
type LoginByPasswordRequest struct {
	Account  string `json:"account" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// UpdateProfileRequest 表示用户资料更新请求。
type UpdateProfileRequest struct {
	Nickname  *string `json:"nickname"`
	AvatarURL *string `json:"avatar_url"`
	Bio       *string `json:"bio"`
}

// tokenResponse 表示登录成功后返回的访问令牌信息。
type tokenResponse struct {
	AccessToken      string `json:"access_token"`
	TokenType        string `json:"token_type"`
	ExpiresInSeconds int64  `json:"expires_in_seconds"`
}

// userProfileResponse 表示对外返回的用户基本资料。
type userProfileResponse struct {
	ID             int64  `json:"id"`
	Account        string `json:"account"`
	Nickname       string `json:"nickname"`
	Role           string `json:"role"`
	Bio            string `json:"bio"`
	Status         int    `json:"status"`
	AvatarURL      string `json:"avatar_url"`
	FollowingCount int    `json:"following_count"`
	FollowerCount  int    `json:"follower_count"`
	WorkCount      int    `json:"work_count"`
}

// publicUserProfileResponse 表示公开用户资料响应，隐藏账号、角色和状态等内部字段。
type publicUserProfileResponse struct {
	ID             int64  `json:"id"`
	Nickname       string `json:"nickname"`
	AvatarURL      string `json:"avatar_url"`
	Bio            string `json:"bio"`
	FollowingCount int    `json:"following_count"`
	FollowerCount  int    `json:"follower_count"`
	WorkCount      int    `json:"work_count"`
}
