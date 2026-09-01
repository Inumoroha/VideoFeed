package feed

import "errors"

var ErrLoadFeedFailed = errors.New("failed to load feed")
var ErrFeedServiceNotInitialized = errors.New("feed service is not initialized")
