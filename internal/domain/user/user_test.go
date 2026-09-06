package user

import "testing"

// TestNewPreservesPasswordWhitespace 验证创建用户时合法密码的首尾空格会被保留。
func TestNewPreservesPasswordWhitespace(t *testing.T) {
	user, err := New("account", " secret ", "nickname")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := user.Authenticate(" secret "); err != nil {
		t.Fatalf("Authenticate() with original password error = %v", err)
	}
	if err := user.Authenticate("secret"); err == nil {
		t.Fatal("Authenticate() accepted a password with missing whitespace")
	}
}

// TestRestoreUserWithStats 验证恢复用户时会保留有效统计值并将负值归零。
func TestRestoreUserWithStats(t *testing.T) {
	user := RestoreUserWithStats(
		1,
		"account",
		"password-hash",
		"nickname",
		RoleUser,
		"",
		"",
		StatusNormal,
		3,
		-1,
		5,
	)

	if user.FollowingCount != 3 {
		t.Fatalf("FollowingCount = %d, want 3", user.FollowingCount)
	}
	if user.FollowerCount != 0 {
		t.Fatalf("FollowerCount = %d, want 0", user.FollowerCount)
	}
	if user.WorkCount != 5 {
		t.Fatalf("WorkCount = %d, want 5", user.WorkCount)
	}
}
