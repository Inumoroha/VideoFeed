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

// tokenResponse 表示登录成功后返回的访问令牌信息。
type tokenResponse struct {
	AccessToken      string `json:"access_token"`
	TokenType        string `json:"token_type"`
	ExpiresInSeconds int64  `json:"expires_in_seconds"`
}

// userProfileResponse 表示对外返回的用户基本资料。
type userProfileResponse struct {
	ID       int64  `json:"id"`
	Account  string `json:"account"`
	Nickname string `json:"nickname"`
	Role     string `json:"role"`
}
