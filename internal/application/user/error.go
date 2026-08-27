package user

import "errors"

var (
	// ErrLoadUserFailed 表示从用户仓储加载用户失败。
	ErrLoadUserFailed = errors.New("failed to load user")
	// ErrSaveUserFailed 表示保存用户到用户仓储失败。
	ErrSaveUserFailed = errors.New("failed to save user")
	// ErrSignAccessTokenFailed 表示签发访问令牌失败。
	ErrSignAccessTokenFailed = errors.New("failed to sign access token")
	// ErrUpdateUserFailed 表示更新用户失败。
	ErrUpdateUserFailed = errors.New("failed to update user")
)
