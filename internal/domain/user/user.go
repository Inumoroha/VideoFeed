package user

import (
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const RoleUser = "user"

type User struct {
	ID       int64
	Account  string
	Password string
	Nickname string
	Role     string
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
	}, nil
}

// RestoreUser 根据持久化数据恢复用户对象，并在角色为空时使用普通用户角色。
func RestoreUser(id int64, account, passwordHash, nickname, role string) *User {
	account = strings.TrimSpace(account)
	passwordHash = strings.TrimSpace(passwordHash)
	nickname = strings.TrimSpace(nickname)
	role = strings.TrimSpace(role)
	if role == "" {
		role = RoleUser
	}

	return &User{
		ID:       id,
		Account:  account,
		Password: passwordHash,
		Nickname: nickname,
		Role:     role,
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
