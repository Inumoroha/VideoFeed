package relation

import "errors"

var (
	// ErrRelationServiceNotInitialized 表示关系应用服务缺少仓储依赖。
	ErrRelationServiceNotInitialized = errors.New("relation service is not initialized")
	// ErrLoadRelationFailed 表示读取关系数据失败。
	ErrLoadRelationFailed = errors.New("failed to load relation")
	// ErrUpdateRelationFailed 表示更新关注关系失败。
	ErrUpdateRelationFailed = errors.New("failed to update relation")
)
