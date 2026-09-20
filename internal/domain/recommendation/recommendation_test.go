package recommendation

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

// TestDomainValidation 验证场景和标识规范化、全部输入边界及非有限游标拒绝。
func TestDomainValidation(t *testing.T) {
	req, err := NewCandidateRequest(42, " Recommend ", " req ", nil, 10)
	if err != nil || req.Scene != "recommend" || req.RequestID != "req" {
		t.Fatal(req, err)
	}
	for _, tc := range []struct {
		user      int64
		scene, id string
		limit     int
		want      error
	}{{0, "x", "", 1, ErrInvalidUserID}, {1, " ", "", 1, ErrEmptyScene}, {1, strings.Repeat("中", 11), "", 1, ErrSceneTooLong}, {1, "x", strings.Repeat("a", 65), 1, ErrRequestIDTooLong}, {1, "x", "", 0, ErrInvalidLimit}, {1, "x", "", 101, ErrInvalidLimit}} {
		if _, err := NewCandidateRequest(tc.user, tc.scene, tc.id, nil, tc.limit); !errors.Is(err, tc.want) {
			t.Fatal(tc, err)
		}
	}
	if _, err := NewExposureWrite(1, 0, "recommend", ""); !errors.Is(err, ErrInvalidVideoID) {
		t.Fatal(err)
	}
	for _, score := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if (&Cursor{VideoID: 1, PublishedAt: time.Now(), RankScore: score}).Valid() {
			t.Fatal(score)
		}
	}
	if _, err := NewCandidateRequest(1, "recommend", "", &Cursor{}, 10); !errors.Is(err, ErrInvalidCursor) {
		t.Fatal(err)
	}
	if write, err := NewExposureWrite(1, 2, " Recommend ", " req "); err != nil || write.Scene != "recommend" || write.RequestID != "req" {
		t.Fatal(write, err)
	}
}
