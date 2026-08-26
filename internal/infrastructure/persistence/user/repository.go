package user

import (
	"context"
	"errors"
	"strings"

	domainuser "github.com/inumoroha/video-feed/internal/domain/user"

	"gorm.io/gorm"
)

type Repository struct {
	gormDB *gorm.DB
}

var _ domainuser.Repository = (*Repository)(nil)

// New 创建基于 GORM 数据库实例的用户仓储。
func New(gormDB *gorm.DB) *Repository {
	return &Repository{gormDB: gormDB}
}

// Save 新增用户或更新已有用户，并将新增记录生成的 ID 回写到领域对象。
func (r *Repository) Save(ctx context.Context, user *domainuser.User) error {
	if r == nil || r.gormDB == nil {
		return errors.New("user repository is not initialized")
	}
	if user == nil {
		return errors.New("user is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	model := UserModel{
		ID:           user.ID,
		Account:      user.Account,
		PasswordHash: user.Password,
		Nickname:     user.Nickname,
		Role:         user.Role,
	}

	query := r.gormDB.WithContext(ctx)
	if user.ID == 0 {
		if err := query.Create(&model).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return domainuser.ErrAccountAlreadyExists
			}
			return err
		}
		user.ID = model.ID
		return nil
	}

	result := query.Model(&UserModel{}).
		Where("id = ?", user.ID).
		Updates(map[string]any{
			"account":  model.Account,
			"password": model.PasswordHash,
			"nickname": model.Nickname,
			"role":     model.Role,
		})
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
			return domainuser.ErrAccountAlreadyExists
		}
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domainuser.ErrUserNotFound
	}
	return nil
}

// FindByAccount 按账号查询用户，并将持久化记录恢复为领域用户对象。
func (r *Repository) FindByAccount(ctx context.Context, account string) (*domainuser.User, error) {
	if r == nil || r.gormDB == nil {
		return nil, errors.New("user repository is not initialized")
	}
	account = strings.TrimSpace(account)
	if account == "" {
		return nil, domainuser.ErrEmptyAccount
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var user UserModel
	err := r.gormDB.WithContext(ctx).Where("account = ?", account).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainuser.ErrUserNotFound
		}
		return nil, err
	}

	return domainuser.RestoreUser(
		user.ID,
		user.Account,
		user.PasswordHash,
		user.Nickname,
		user.Role,
	), nil
}
