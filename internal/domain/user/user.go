package user

import (
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const (
	RoleUser     = "user"
	RoleAdmin    = "admin"
	StatusNormal = 1
)

// User 表示用户账户资料、认证凭据和个人页统计信息。
type User struct {
	ID        int64
	Account   string
	Password  string
	Nickname  string
	Role      string
	AvatarURL string
	Bio       string
	Status    int
	// FollowingCount 和 FollowerCount 来自关系模块统计表，用于个人页展示。
	FollowingCount int
	FollowerCount  int
	WorkCount      int
}

// New 校验用户输入并生成密码哈希，返回一个默认角色为普通用户的新用户。
func New(account, password, nickname string) (*User, error) {
	account = strings.TrimSpace(account)
	nickname = strings.TrimSpace(nickname)

	if account == "" {
		return nil, ErrEmptyAccount
	}
	if strings.TrimSpace(password) == "" {
		return nil, ErrEmptyPassword
	}
	if nickname == "" {
		return nil, ErrEmptyNickname
	}

	hashPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, ErrHashPasswordFailed
	}

	return &User{
		Account:  account,
		Password: string(hashPassword),
		Nickname: nickname,
		Role:     RoleUser,
		Status:   StatusNormal,
	}, nil
}

// RestoreUser 根据持久化数据恢复用户对象，并在角色为空时使用普通用户角色。
func RestoreUser(id int64, account, passwordHash, nickname, role, avatarURL, bio string, status int) *User {
	return RestoreUserWithStats(id, account, passwordHash, nickname, role, avatarURL, bio, status, 0, 0, 0)
}

// RestoreUserWithStats 根据持久化数据恢复用户对象，并在角色为空时使用普通用户角色，同时设置关注数、粉丝数和作品数。
func RestoreUserWithStats(id int64, account, passwordHash, nickname, role, avatarURL, bio string, status int, followingCount, followerCount, workCount int) *User {
	account = strings.TrimSpace(account)
	passwordHash = strings.TrimSpace(passwordHash)
	nickname = strings.TrimSpace(nickname)
	role = strings.TrimSpace(role)
	avatarURL = strings.TrimSpace(avatarURL)
	bio = strings.TrimSpace(bio)
	if status == 0 {
		status = StatusNormal
	}
	if role == "" {
		role = RoleUser
	}

	return &User{
		ID:             id,
		Account:        account,
		Password:       passwordHash,
		Nickname:       nickname,
		Role:           role,
		AvatarURL:      avatarURL,
		Bio:            bio,
		Status:         status,
		FollowingCount: clampCount(followingCount),
		FollowerCount:  clampCount(followerCount),
		WorkCount:      clampCount(workCount),
	}
}

// Authenticate 使用用户保存的密码哈希验证输入密码。
func (u *User) Authenticate(password string) error {
	if strings.TrimSpace(password) == "" {
		return ErrEmptyPassword
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password)); err != nil {
		return ErrInvalidCredentials
	}
	return nil
}

// UpdateProfile 按传入字段更新用户资料，昵称不能仅由空白字符组成，头像地址和简介会去除首尾空白。
func (u *User) UpdateProfile(nickname, avatarURL, bio *string) error {
	if nickname == nil && avatarURL == nil && bio == nil {
		return ErrEmptyProfileUpdate
	}

	if nickname != nil {
		value := strings.TrimSpace(*nickname)
		if value == "" {
			return ErrEmptyNickname
		}
		u.Nickname = value
	}
	if avatarURL != nil {
		u.AvatarURL = strings.TrimSpace(*avatarURL)
	}
	if bio != nil {
		u.Bio = strings.TrimSpace(*bio)
	}

	return nil
}

// clampCount 确保计数值不为负数。
func clampCount(value int) int {
	if value < 0 {
		return 0
	}
	return value
}
