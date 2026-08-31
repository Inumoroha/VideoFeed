package feed

import "errors"

var ErrLoadFeedFailed = errors.New("failed to load feed")
var ErrSaveTimeCursorFailed = errors.New("failed to save feed cursor")
var ErrLoadTimeCursorFailed = errors.New("failed to load feed cursor")
var ErrSaveViewEventFailed = errors.New("failed to save view event")
var ErrLoadViewEventFailed = errors.New("failed to load view event")
var ErrFeedServiceNotInitialized = errors.New("feed service is not initialized")
