package user

// LoginResult 表示用户登录成功后返回的访问令牌信息。
type LoginResult struct {
	AccessToken      string
	TokenType        string
	ExpiresInSeconds int64
}

// Profile 表示可向调用方返回的用户基本资料。
type Profile struct {
	ID       int64
	Account  string
	Nickname string
	Role     string
	Bio      string
	Status   int
	AvatarURL string
}
