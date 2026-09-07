package user

import (
	"context"
	"errors"
	"strings"

	domainuser "github.com/inumoroha/video-feed/internal/domain/user"
	domainvideo "github.com/inumoroha/video-feed/internal/domain/video"

	"gorm.io/gorm"
)

// userWithStatModel 是包含用户信息和统计数据的持久化模型。
type userWithStatModel struct {
	ID             int64  `gorm:"column:id"`
	Account        string `gorm:"column:account"`
	PasswordHash   string `gorm:"column:password"`
	Nickname       string `gorm:"column:nickname"`
	AvatarURL      string `gorm:"column:avatar_url"`
	Bio            string `gorm:"column:bio"`
	Status         int    `gorm:"column:status"`
	Role           string `gorm:"column:role"`
	FollowingCount int    `gorm:"column:following_count"`
	FollowerCount  int    `gorm:"column:follower_count"`
	WorkCount      int    `gorm:"column:work_count"`
}
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
		AvatarURL:    user.AvatarURL,
		Bio:          user.Bio,
		Status:       user.Status,
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
			"account":    model.Account,
			"password":   model.PasswordHash,
			"nickname":   model.Nickname,
			"role":       model.Role,
			"avatar_url": model.AvatarURL,
			"bio":        model.Bio,
			"status":     model.Status,
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
		user.AvatarURL,
		user.Bio,
		user.Status,
	), nil
}

// FindByID 按用户 ID 查询持久化记录和已发布作品数，并将其恢复为领域用户对象。
func (r *Repository) FindByID(ctx context.Context, id int64) (*domainuser.User, error) {
	if r == nil || r.gormDB == nil {
		return nil, errors.New("user repository is not initialized")
	}
	if id <= 0 {
		return nil, domainuser.ErrInvalidUserID
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var user userWithStatModel
	err := r.gormDB.WithContext(ctx).
		Table("users AS u").
		Select(userWithStatSelect()).
		Joins("LEFT JOIN (SELECT author_id, COUNT(*) AS work_count FROM videos WHERE status = ? GROUP BY author_id) AS published_works ON published_works.author_id = u.id", domainvideo.StatusPublished).
		Where("u.id = ?", id).
		Take(&user).
		Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainuser.ErrUserNotFound
		}
		return nil, err
	}

	return restoreUser(user), nil
}

// UpdateProfile 按用户 ID 更新昵称、头像地址和简介，并将不存在的用户映射为领域错误。
func (r *Repository) UpdateProfile(ctx context.Context, user *domainuser.User) error {
	if r == nil || r.gormDB == nil {
		return errors.New("user repository is not initialized")
	}
	if user == nil {
		return errors.New("user is nil")
	}
	if user.ID <= 0 {
		return domainuser.ErrInvalidUserID
	}
	if ctx == nil {
		ctx = context.Background()
	}

	result := r.gormDB.WithContext(ctx).
		Model(&UserModel{}).
		Where("id = ?", user.ID).
		Updates(map[string]any{
			"nickname":   user.Nickname,
			"avatar_url": user.AvatarURL,
			"bio":        user.Bio,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domainuser.ErrUserNotFound
	}
	return nil
}

// restoreUser 将包含统计信息的持久化查询结果转换为领域用户对象。
func restoreUser(user userWithStatModel) *domainuser.User {
	return domainuser.RestoreUserWithStats(
		user.ID,
		user.Account,
		user.PasswordHash,
		user.Nickname,
		user.Role,
		user.AvatarURL,
		user.Bio,
		user.Status,
		user.FollowingCount,
		user.FollowerCount,
		user.WorkCount,
	)
}

// userWithStatSelect 返回用户和已发布作品统计的 SELECT 子句；关注统计未实现时返回零。
func userWithStatSelect() string {
	return "u.id, u.account, u.password, u.nickname, u.avatar_url, u.bio, u.status, u.role, 0 AS following_count, 0 AS follower_count, COALESCE(published_works.work_count, 0) AS work_count"
}
