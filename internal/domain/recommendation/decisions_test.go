package recommendation

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestExposureDecisionRequestValidation 验证规范化、首次顺序、输入独立性以及按字节限制的边界和非法标识。
func TestExposureDecisionRequestValidation(t *testing.T) {
	ids := []int64{3, 1, 3, 2}
	result, err := NewExposureDecisionRequest(42, " ReCommEnd ", " req ", ids)
	if err != nil || result.Scene != "recommend" || result.RequestID != "req" || !reflect.DeepEqual(result.VideoIDs, []int64{3, 1, 2}) {
		t.Fatal(result, err)
	}
	ids[0] = 99
	if result.VideoIDs[0] != 3 {
		t.Fatal("请求共享输入切片")
	}
	for _, tc := range []struct {
		user           int64
		scene, request string
		ids            []int64
		want           error
	}{
		{0, "recommend", "", nil, ErrInvalidUserID}, {-1, "recommend", "", nil, ErrInvalidUserID},
		{1, " \t ", "", nil, ErrEmptyScene}, {1, strings.Repeat("a", 33), "", nil, ErrSceneTooLong},
		{1, strings.Repeat("中", 11), "", nil, ErrSceneTooLong}, {1, "r", strings.Repeat("a", 65), nil, ErrRequestIDTooLong},
		{1, "r", strings.Repeat("中", 22), nil, ErrRequestIDTooLong}, {1, "r", "", []int64{1, 0}, ErrInvalidVideoID},
		{1, "r", "", []int64{-1}, ErrInvalidVideoID}, {1, strings.Repeat("a", 32), strings.Repeat("a", 64), nil, nil},
	} {
		got, err := NewExposureDecisionRequest(tc.user, tc.scene, tc.request, tc.ids)
		if !errors.Is(err, tc.want) {
			t.Fatalf("%+v: %v", tc, err)
		}
		if err == nil && got.VideoIDs == nil {
			t.Fatal("空列表必须返回空切片")
		}
	}
}

// TestRestoreExposureDecisionCopy 验证原因规范化、空时间以及时间指针复制。
func TestRestoreExposureDecisionCopy(t *testing.T) {
	now := time.Now()
	original := now
	decision := RestoreExposureDecision(3, false, " recently_exposed ", &now)
	now = now.Add(time.Hour)
	if decision.Reason != ExposureDecisionReasonRecentlyExposed || !decision.LastExposedAt.Equal(original) {
		t.Fatal(decision)
	}
	if RestoreExposureDecision(4, true, " fresh ", nil).LastExposedAt != nil {
		t.Fatal("未曝光时间应为空")
	}
}
