package infrarelation

import (
	"context"
	"errors"
	"strings"
	"time"

	domainrelation "github.com/inumoroha/video-feed/internal/domain/relation"
	domainuser "github.com/inumoroha/video-feed/internal/domain/user"
	infrauser "github.com/inumoroha/video-feed/internal/infrastructure/persistence/user"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository 使用 GORM 持久化关注关系、统计和幂等结果。
type Repository struct {
	gormDB *gorm.DB
}

// relationUserModel 保存关联用户表后的关注列表查询结果。
type relationUserModel struct {
	UserID     int64     `gorm:"column:user_id"`
	Nickname   string    `gorm:"column:nickname"`
	AvatarURL  string    `gorm:"column:avatar_url"`
	Bio        string    `gorm:"column:bio"`
	FollowedAt time.Time `gorm:"column:followed_at"`
}

// New 创建关系仓储实现。
func New(db *gorm.DB) *Repository {
	return &Repository{gormDB: db}
}

// SetFollow 设置关注或取关状态，在同一事务中维护双方统计并保存首次幂等结果。
func (r *Repository) SetFollow(ctx context.Context, userID int64, targetUserID int64, active bool, idempotencyKey string) (*domainrelation.Follow, *domainrelation.RelationStat, *domainrelation.RelationStat, error) {
	if err := r.validateRepository(); err != nil {
		return nil, nil, nil, err
	}

	request, err := domainrelation.NewFollow(userID, targetUserID, idempotencyKey)
	if err != nil {
		return nil, nil, nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	idempotencyKey = request.IdempotencyKey
	var follow FollowModel
	var userStat RelationStatModel
	var targetStat RelationStatModel
	err = r.gormDB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if idempotencyKey != "" {
			var stored FollowIdempotencyModel
			lookupErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("user_id = ? AND idempotency_key = ?", userID, idempotencyKey).
				Take(&stored).
				Error
			if lookupErr == nil {
				follow, userStat, targetStat = restoreIdempotencyResult(stored)
				return nil
			}
			if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
				return lookupErr
			}
		}

		if err := lockNormalUsers(tx, userID, targetUserID); err != nil {
			return err
		}
		for _, id := range orderedUserIDs(userID, targetUserID) {
			if err := ensureStat(tx, id); err != nil {
				return err
			}
		}

		findErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND target_user_id = ?", userID, targetUserID).
			Take(&follow).
			Error
		if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return findErr
		}

		nextStatus := statusFromActive(active)
		delta := 0
		if errors.Is(findErr, gorm.ErrRecordNotFound) {
			follow = FollowModel{
				UserID:         userID,
				TargetUserID:   targetUserID,
				Status:         nextStatus,
				IdempotencyKey: idempotencyKeyPtr(idempotencyKey),
			}
			if err := tx.Create(&follow).Error; err != nil {
				return err
			}
			if active {
				delta = 1
			}
		} else {
			previousStatus := follow.Status
			previousIdempotencyKey := idempotencyKeyValue(follow.IdempotencyKey)
			if !(idempotencyKey != "" && previousIdempotencyKey == idempotencyKey) {
				follow.Status = nextStatus
				follow.IdempotencyKey = idempotencyKeyPtr(idempotencyKey)
				previousActive := previousStatus == domainrelation.FollowStatusActive
				if previousActive != active {
					if active {
						delta = 1
					} else {
						delta = -1
					}
				}
				if previousStatus != follow.Status || previousIdempotencyKey != idempotencyKey {
					if err := tx.Save(&follow).Error; err != nil {
						return err
					}
				}
			}
		}

		var statErr error
		if delta != 0 {
			userStat, statErr = updateStat(tx, userID, delta, 0)
			if statErr != nil {
				return statErr
			}
			targetStat, statErr = updateStat(tx, targetUserID, 0, delta)
			if statErr != nil {
				return statErr
			}
		} else {
			userStat, statErr = currentStat(tx, userID)
			if statErr != nil {
				return statErr
			}
			targetStat, statErr = currentStat(tx, targetUserID)
			if statErr != nil {
				return statErr
			}
		}

		if idempotencyKey == "" {
			return nil
		}
		return tx.Create(newIdempotencyModel(idempotencyKey, follow, userStat, targetStat)).Error
	})
	if err != nil {
		if idempotencyKey != "" && isDuplicateKeyError(err) {
			return r.loadFollowIdempotencyResult(ctx, userID, idempotencyKey)
		}
		return nil, nil, nil, err
	}

	return restoreFollow(follow), restoreStat(userStat), restoreStat(targetStat), nil
}

// ListFollowing 按关注时间和用户 ID 倒序查询当前用户关注的正常用户。
func (r *Repository) ListFollowing(ctx context.Context, userID int64, cursor *domainrelation.ListCursor, limit int) ([]*domainrelation.UserItem, error) {
	limit, err := r.validateListRequest(userID, cursor, limit)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	query := r.gormDB.WithContext(ctx).
		Table("user_follow AS f").
		Select("a.id AS user_id, a.nickname, a.avatar_url, a.bio, f.updated_at AS followed_at").
		Joins("JOIN users AS a ON a.id = f.target_user_id").
		Where("f.user_id = ? AND f.status = ? AND a.status = ?", userID, domainrelation.FollowStatusActive, domainuser.StatusNormal)
	if cursor != nil {
		query = query.Where("(f.updated_at < ? OR (f.updated_at = ? AND f.target_user_id < ?))", cursor.FollowedAt, cursor.FollowedAt, cursor.UserID)
	}

	return scanUserItems(query.Order("f.updated_at DESC").Order("f.target_user_id DESC").Limit(limit))
}

// ListFollowers 按关注时间和用户 ID 倒序查询关注当前用户的正常用户。
func (r *Repository) ListFollowers(ctx context.Context, userID int64, cursor *domainrelation.ListCursor, limit int) ([]*domainrelation.UserItem, error) {
	limit, err := r.validateListRequest(userID, cursor, limit)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	query := r.gormDB.WithContext(ctx).
		Table("user_follow AS f").
		Select("a.id AS user_id, a.nickname, a.avatar_url, a.bio, f.updated_at AS followed_at").
		Joins("JOIN users AS a ON a.id = f.user_id").
		Where("f.target_user_id = ? AND f.status = ? AND a.status = ?", userID, domainrelation.FollowStatusActive, domainuser.StatusNormal)
	if cursor != nil {
		query = query.Where("(f.updated_at < ? OR (f.updated_at = ? AND f.user_id < ?))", cursor.FollowedAt, cursor.FollowedAt, cursor.UserID)
	}

	return scanUserItems(query.Order("f.updated_at DESC").Order("f.user_id DESC").Limit(limit))
}

// scanUserItems 将用户列表查询结果恢复为领域展示对象。
func scanUserItems(query *gorm.DB) ([]*domainrelation.UserItem, error) {
	var models []relationUserModel
	if err := query.Scan(&models).Error; err != nil {
		return nil, err
	}

	items := make([]*domainrelation.UserItem, 0, len(models))
	for _, model := range models {
		items = append(items, domainrelation.RestoreUserItem(model.UserID, model.Nickname, model.AvatarURL, model.Bio, model.FollowedAt))
	}
	return items, nil
}

// validateRepository 校验关系仓储及其数据库实例已初始化。
func (r *Repository) validateRepository() error {
	if r == nil || r.gormDB == nil {
		return errors.New("relation repository is not initialized")
	}
	return nil
}

// validateListRequest 校验并规范化关系列表查询的标识、游标和数量。
func (r *Repository) validateListRequest(userID int64, cursor *domainrelation.ListCursor, limit int) (int, error) {
	if err := r.validateRepository(); err != nil {
		return 0, err
	}
	if userID <= 0 {
		return 0, domainrelation.ErrInvalidUserID
	}
	if limit <= 0 {
		return 0, domainrelation.ErrInvalidLimit
	}
	if limit > domainrelation.MaxLimit+1 {
		limit = domainrelation.MaxLimit + 1
	}
	if cursor != nil && (cursor.FollowedAt.IsZero() || cursor.UserID <= 0) {
		return 0, domainrelation.ErrInvalidCursor
	}
	return limit, nil
}

// lockNormalUsers 按用户 ID 升序锁定发起用户和目标用户，避免双向关注请求形成死锁。
func lockNormalUsers(tx *gorm.DB, userID int64, targetUserID int64) error {
	for _, id := range orderedUserIDs(userID, targetUserID) {
		if err := lockNormalUser(tx, id); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if id == targetUserID {
					return domainrelation.ErrTargetUserNotFound
				}
				return domainrelation.ErrUserNotFound
			}
			return err
		}
	}
	return nil
}

// orderedUserIDs 返回两个不同用户 ID 的升序数组，用于统一事务加锁顺序。
func orderedUserIDs(userID int64, targetUserID int64) [2]int64 {
	if userID < targetUserID {
		return [2]int64{userID, targetUserID}
	}
	return [2]int64{targetUserID, userID}
}

// lockNormalUser 锁定一条状态正常的用户记录。
func lockNormalUser(tx *gorm.DB, userID int64) error {
	var user infrauser.UserModel
	return tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND status = ?", userID, domainuser.StatusNormal).
		Take(&user).
		Error
}

// ensureStat 为用户创建统计记录，已存在时保留当前记录。
func ensureStat(tx *gorm.DB, userID int64) error {
	stat := RelationStatModel{UserID: userID}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&stat).Error
}

// currentStat 锁定并读取用户当前的关系统计记录。
func currentStat(tx *gorm.DB, userID int64) (RelationStatModel, error) {
	var stat RelationStatModel
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ?", userID).Take(&stat).Error
	return stat, err
}

// updateStat 在锁定统计记录后更新关注数和粉丝数，并防止结果小于零。
func updateStat(tx *gorm.DB, userID int64, followingDelta int, followerDelta int) (RelationStatModel, error) {
	stat, err := currentStat(tx, userID)
	if err != nil {
		return stat, err
	}
	stat.FollowingCount = clampCount(stat.FollowingCount + followingDelta)
	stat.FollowerCount = clampCount(stat.FollowerCount + followerDelta)
	if err := tx.Save(&stat).Error; err != nil {
		return stat, err
	}
	return stat, nil
}

// newIdempotencyModel 根据首次执行的关注和统计结果构造幂等快照。
func newIdempotencyModel(idempotencyKey string, follow FollowModel, userStat RelationStatModel, targetStat RelationStatModel) *FollowIdempotencyModel {
	return &FollowIdempotencyModel{
		UserID:               follow.UserID,
		IdempotencyKey:       idempotencyKey,
		TargetUserID:         follow.TargetUserID,
		FollowID:             follow.ID,
		Active:               follow.Status == domainrelation.FollowStatusActive,
		FollowCreatedAt:      follow.CreatedAt,
		FollowUpdatedAt:      follow.UpdatedAt,
		UserFollowingCount:   userStat.FollowingCount,
		UserFollowerCount:    userStat.FollowerCount,
		UserStatCreatedAt:    userStat.CreatedAt,
		UserStatUpdatedAt:    userStat.UpdatedAt,
		TargetFollowingCount: targetStat.FollowingCount,
		TargetFollowerCount:  targetStat.FollowerCount,
		TargetStatCreatedAt:  targetStat.CreatedAt,
		TargetStatUpdatedAt:  targetStat.UpdatedAt,
	}
}

// restoreIdempotencyResult 将保存的首次请求结果恢复为关注关系和双方统计。
func restoreIdempotencyResult(model FollowIdempotencyModel) (FollowModel, RelationStatModel, RelationStatModel) {
	follow := FollowModel{
		ID:             model.FollowID,
		UserID:         model.UserID,
		TargetUserID:   model.TargetUserID,
		Status:         statusFromActive(model.Active),
		IdempotencyKey: idempotencyKeyPtr(model.IdempotencyKey),
		CreatedAt:      model.FollowCreatedAt,
		UpdatedAt:      model.FollowUpdatedAt,
	}
	userStat := RelationStatModel{
		UserID:         model.UserID,
		FollowingCount: model.UserFollowingCount,
		FollowerCount:  model.UserFollowerCount,
		CreatedAt:      model.UserStatCreatedAt,
		UpdatedAt:      model.UserStatUpdatedAt,
	}
	targetStat := RelationStatModel{
		UserID:         model.TargetUserID,
		FollowingCount: model.TargetFollowingCount,
		FollowerCount:  model.TargetFollowerCount,
		CreatedAt:      model.TargetStatCreatedAt,
		UpdatedAt:      model.TargetStatUpdatedAt,
	}
	return follow, userStat, targetStat
}

// loadFollowIdempotencyResult 读取提交成功的幂等快照，用于处理并发唯一键冲突。
func (r *Repository) loadFollowIdempotencyResult(ctx context.Context, userID int64, idempotencyKey string) (*domainrelation.Follow, *domainrelation.RelationStat, *domainrelation.RelationStat, error) {
	var model FollowIdempotencyModel
	err := r.gormDB.WithContext(ctx).Where("user_id = ? AND idempotency_key = ?", userID, idempotencyKey).Take(&model).Error
	if err != nil {
		return nil, nil, nil, err
	}
	follow, userStat, targetStat := restoreIdempotencyResult(model)
	return restoreFollow(follow), restoreStat(userStat), restoreStat(targetStat), nil
}

// restoreFollow 将持久化关注关系转换为领域对象。
func restoreFollow(model FollowModel) *domainrelation.Follow {
	return domainrelation.RestoreFollow(model.ID, model.UserID, model.TargetUserID, model.Status, idempotencyKeyValue(model.IdempotencyKey), model.CreatedAt, model.UpdatedAt)
}

// restoreStat 将持久化关系统计转换为领域对象。
func restoreStat(model RelationStatModel) *domainrelation.RelationStat {
	return domainrelation.RestoreRelationStat(model.UserID, model.FollowingCount, model.FollowerCount, model.CreatedAt, model.UpdatedAt)
}

// statusFromActive 将显式关注状态转换为持久化状态值。
func statusFromActive(active bool) int {
	if active {
		return domainrelation.FollowStatusActive
	}
	return domainrelation.FollowStatusCanceled
}

// idempotencyKeyPtr 将空幂等键转换为 nil，避免将空字符串当作有效请求键。
func idempotencyKeyPtr(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

// idempotencyKeyValue 将可空幂等键转换为普通字符串。
func idempotencyKeyValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// clampCount 将统计数量限制为不小于零。
func clampCount(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

// isDuplicateKeyError 判断错误是否为 PostgreSQL 唯一键冲突。
func isDuplicateKeyError(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var postgresErr *pgconn.PgError
	return errors.As(err, &postgresErr) && postgresErr.Code == "23505"
}

var _ domainrelation.Repository = (*Repository)(nil)
