package video

import (
	domainvideo "github.com/inumoroha/video-feed/internal/domain/video"
)

type CreateResult struct {
	Video   *domainvideo.Video
	Created bool
}