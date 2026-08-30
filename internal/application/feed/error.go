package feed

import "errors"

var ErrLoadFeedFailed = errors.New("failed to load feed")
var ErrSaveViewEventFailed = errors.New("failed to save view event")
var ErrLoadViewEventFailed = errors.New("failed to load view event")
var ErrFeedServiceNotInitialized = errors.New("feed service is not initialized")
