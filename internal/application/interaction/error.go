package interaction

import "errors"

var ErrLoadInteractionFailed = errors.New("failed to load interaction")
var ErrSaveInteractionFailed = errors.New("failed to save interaction")
var ErrUpdateInteractionFailed = errors.New("failed to update interaction")
var ErrInteractionServiceNotInitialized = errors.New("interaction service is not initialized")
