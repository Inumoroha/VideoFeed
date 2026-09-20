package recommendation

import (
	"context"
	"encoding/json"
	"errors"
	domainembedding "github.com/inumoroha/video-feed/internal/domain/embedding"
	domainexposure "github.com/inumoroha/video-feed/internal/domain/exposure"
	domainrecommendation "github.com/inumoroha/video-feed/internal/domain/recommendation"
	domainvideo "github.com/inumoroha/video-feed/internal/domain/video"
	infraexposure "github.com/inumoroha/video-feed/internal/infrastructure/persistence/exposure"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const hotScoreExpression = "COALESCE(vs.like_count, 0) * 3 + COALESCE(vs.comment_count, 0) * 5 + COALESCE(vs.favorite_count, 0) * 4"
const recentExposureWindow = 7 * 24 * time.Hour
const positiveEventWindow = 30 * 24 * time.Hour

// Repository 通过 PostgreSQL 查询推荐候选和兴趣向量，并以事务保存曝光。
type Repository struct {
	db *gorm.DB
}

// candidateModel 接收候选查询的视频、作者、热度和发布时间。
type candidateModel struct {
	VideoID     int64
	AuthorID    int64
	HotScore    int
	PublishedAt time.Time
}

// videoVectorModel 接收视频向量查询结果。
type videoVectorModel struct {
	VideoID       int64
	EmbeddingJSON string
}

// New 创建使用指定依赖的推荐组件。
func New(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// ListCandidatePool 查询已发布且最近七天未曝光的视频，按热度、发布时间和标识倒序返回。
func (r *Repository) ListCandidatePool(ctx context.Context, userID int64, limit int) ([]*domainrecommendation.Candidate, error) {
	if limit <= 0 {
		return []*domainrecommendation.Candidate{}, nil
	}

	var models []candidateModel
	err := r.db.WithContext(ctx).
		Table("videos AS v").
		Select("v.id AS video_id, v.author_id, ("+hotScoreExpression+") AS hot_score, v.published_at").
		Joins("LEFT JOIN video_stat AS vs ON vs.video_id = v.id").
		Joins(
			"LEFT JOIN exposures AS e ON e.user_id = ? AND e.video_id = v.id AND e.last_exposed_at >= ?",
			userID,
			time.Now().Add(-recentExposureWindow),
		).
		Where("v.status = ? AND v.published_at IS NOT NULL AND e.video_id IS NULL", domainvideo.StatusPublished).
		Order("hot_score DESC").
		Order("v.published_at DESC").
		Order("v.id DESC").
		Limit(limit).
		Scan(&models).
		Error
	if err != nil {
		return nil, err
	}

	candidates := make([]*domainrecommendation.Candidate, 0, len(models))
	for _, model := range models {
		candidates = append(candidates, domainrecommendation.RestoreCandidate(
			model.VideoID,
			model.AuthorID,
			0,
			0,
			model.HotScore,
			0,
			"",
			model.PublishedAt,
		))
	}
	return candidates, nil
}

// LoadUserInterestVector 读取最近三十天最多二百条正向事件并加权向量，跳过无效向量，关闭结果集并传播读取错误。
func (r *Repository) LoadUserInterestVector(ctx context.Context, userID int64) ([]float64, bool, error) {
	rows, err := r.db.WithContext(ctx).
		Table("video_view_events AS ev").
		Select("ve.embedding_json, ev.event_type, ev.watch_ms, ev.completed").
		Joins("JOIN video_embedding AS ve ON ve.video_id = ev.video_id AND ve.model = ?", domainembedding.HashNgramModel).
		Where("ev.user_id = ? AND ev.created_at >= ? AND ev.event_type IN ?", userID, time.Now().Add(-positiveEventWindow), positiveEventTypes()).
		Order("ev.created_at DESC").
		Limit(200).
		Rows()
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	var sum []float64
	var totalWeight float64
	for rows.Next() {
		var embeddingJSON string
		var eventType string
		var watchMs int
		var completed bool
		if err := rows.Scan(&embeddingJSON, &eventType, &watchMs, &completed); err != nil {
			return nil, false, err
		}
		vector, err := decodeVector(embeddingJSON)
		if err != nil || len(vector) == 0 {
			continue
		}
		if len(sum) == 0 {
			sum = make([]float64, len(vector))
		}
		if len(vector) != len(sum) {
			continue
		}
		weight := eventWeight(eventType, watchMs, completed)
		for i := range vector {
			sum[i] += vector[i] * weight
		}
		totalWeight += weight
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	if len(sum) == 0 || totalWeight == 0 {
		return nil, false, nil
	}
	for i := range sum {
		sum[i] = sum[i] / totalWeight
	}
	return sum, true, nil
}

// LoadVideoVectors 批量读取指定模型的视频向量，跳过无法解码的记录。
func (r *Repository) LoadVideoVectors(ctx context.Context, videoIDs []int64) (map[int64][]float64, error) {
	vectors := map[int64][]float64{}
	if len(videoIDs) == 0 {
		return vectors, nil
	}

	var models []videoVectorModel
	err := r.db.WithContext(ctx).
		Table("video_embedding").
		Select("video_id, embedding_json").
		Where("video_id IN ? AND model = ?", videoIDs, domainembedding.HashNgramModel).
		Scan(&models).
		Error
	if err != nil {
		return nil, err
	}
	for _, model := range models {
		vector, err := decodeVector(model.EmbeddingJSON)
		if err != nil {
			continue
		}
		vectors[model.VideoID] = vector
	}
	return vectors, nil
}

// SaveExposures 在单一事务中校验视频、追加流水并原子累加曝光，任一步失败回滚整个批次；跨请求不去重。
func (r *Repository) SaveExposures(ctx context.Context, writes []*domainrecommendation.ExposureWrite) ([]*domainrecommendation.Exposure, error) {
	if len(writes) == 0 {
		return []*domainrecommendation.Exposure{}, nil
	}

	exposures := make([]*domainrecommendation.Exposure, 0, len(writes))
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, write := range writes {
			if write == nil {
				continue
			}
			if err := ensurePublishedVideo(tx, write.VideoID); err != nil {
				return err
			}
			event := infraexposure.ViewEventModel{
				UserID:    write.UserID,
				VideoID:   write.VideoID,
				Scene:     write.Scene,
				RequestID: stringPtr(write.RequestID),
				EventType: domainexposure.EventTypeExposed,
				WatchMs:   0,
				Completed: false,
			}
			if err := tx.Create(&event).Error; err != nil {
				return err
			}
			model := infraexposure.ExposureModel{
				UserID:         write.UserID,
				VideoID:        write.VideoID,
				FirstExposedAt: event.CreatedAt,
				LastExposedAt:  event.CreatedAt,
				ExposureCount:  1,
				LastScene:      write.Scene,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{
					{Name: "user_id"},
					{Name: "video_id"},
				},
				DoUpdates: clause.Assignments(map[string]any{
					"last_exposed_at": gorm.Expr("EXCLUDED.last_exposed_at"),
					"exposure_count":  gorm.Expr("exposures.exposure_count + 1"),
					"last_scene":      gorm.Expr("EXCLUDED.last_scene"),
					"updated_at":      gorm.Expr("EXCLUDED.updated_at"),
				}),
			}).Create(&model).Error; err != nil {
				return err
			}

			var saved infraexposure.ExposureModel
			if err := tx.Where("user_id = ? AND video_id = ?", write.UserID, write.VideoID).Take(&saved).Error; err != nil {
				return err
			}
			exposures = append(exposures, restoreExposure(saved))
		}
		return nil
	})
	if err != nil {
		return nil, mapRecommendationError(err)
	}
	return exposures, nil
}

// decodeVector 将 JSON 数组解码为浮点向量。
func decodeVector(content string) ([]float64, error) {
	var vector []float64
	if err := json.Unmarshal([]byte(content), &vector); err != nil {
		return nil, err
	}
	return vector, nil
}

// positiveEventTypes 返回用于画像的播放和完播事件类型。
func positiveEventTypes() []string {
	return []string{
		domainexposure.EventTypePlay,
		domainexposure.EventTypeComplete,
	}
}

// eventWeight 根据事件类型、观看时长和完成标记计算兴趣权重。
func eventWeight(eventType string, watchMs int, completed bool) float64 {
	switch eventType {
	case domainexposure.EventTypeComplete:
		return 3
	case domainexposure.EventTypePlay:
		weight := 1 + float64(watchMs)/30000
		if weight > 2 {
			weight = 2
		}
		if completed {
			weight += 1
		}
		return weight
	default:
		return 1
	}
}

// ensurePublishedVideo 检查视频是否已发布，不存在或未发布时返回领域错误。
func ensurePublishedVideo(tx *gorm.DB, videoID int64) error {
	var item struct {
		ID int64
	}
	err := tx.Table("videos").
		Select("id").
		Where("id = ? AND status = ?", videoID, domainvideo.StatusPublished).
		Take(&item).
		Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domainrecommendation.ErrVideoNotFound
		}
		return err
	}
	return nil
}

// restoreExposure 将数据库曝光模型还原为推荐领域聚合。
func restoreExposure(model infraexposure.ExposureModel) *domainrecommendation.Exposure {
	return domainrecommendation.RestoreExposure(
		model.ID,
		model.UserID,
		model.VideoID,
		model.FirstExposedAt,
		model.LastExposedAt,
		model.ExposureCount,
		model.LastScene,
	)
}

// mapRecommendationError 保留视频不存在及其他仓储错误。
func mapRecommendationError(err error) error {
	if errors.Is(err, domainrecommendation.ErrVideoNotFound) {
		return err
	}
	return err
}

// stringPtr 将空字符串转换为空指针，其余返回字符串指针。
func stringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
