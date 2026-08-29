package video

import "errors"

var ErrLoadVideoFailed = errors.New("failed to load video")
var ErrSaveVideoFailed = errors.New("failed to save video")
var ErrUpdateVideoFailed = errors.New("failed to update video")
var ErrVideoServiceNotInitialized = errors.New("video service is not initialized")
