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
