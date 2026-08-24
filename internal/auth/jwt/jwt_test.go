package jwt

import (
	"errors"
	"testing"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v4"
	"github.com/inumoroha/video-feed/internal/config"
)

// TestManagerSignsAndParsesTokens 验证管理器能够签发并解析访问令牌和刷新令牌。
func TestManagerSignsAndParsesTokens(t *testing.T) {
	manager, err := NewManager(testJWTConfig())
	if err != nil {
		t.Fatalf("create manager: %v", err)
	}

	accessToken, err := manager.SignAccessToken(42, "admin")
	if err != nil {
		t.Fatalf("sign access token: %v", err)
	}
	accessClaims, err := manager.ParseAndValidateToken(accessToken, TokenTypeAccess)
	if err != nil {
		t.Fatalf("parse access token: %v", err)
	}
	if accessClaims.UserID != 42 || accessClaims.Role != "admin" || accessClaims.TokenType != TokenTypeAccess {
		t.Fatalf("unexpected access claims: %+v", accessClaims)
	}

	refreshToken, err := manager.SignRefreshToken(42, "admin")
	if err != nil {
		t.Fatalf("sign refresh token: %v", err)
	}
	refreshClaims, err := manager.ParseAndValidateToken(refreshToken, TokenTypeRefresh)
	if err != nil {
		t.Fatalf("parse refresh token: %v", err)
	}
	if refreshClaims.TokenType != TokenTypeRefresh {
		t.Fatalf("unexpected refresh token type: %q", refreshClaims.TokenType)
	}
}

// TestManagerRejectsInvalidConfiguration 验证管理器拒绝弱密钥和无效有效期配置。
func TestManagerRejectsInvalidConfiguration(t *testing.T) {
	cases := []struct {
		name string
		cfg  config.JWTConfig
		want error
	}{
		{name: "weak secret", cfg: config.JWTConfig{Secret: "short", AccessTTL: time.Minute, RefreshTTL: time.Hour}, want: ErrWeakJWTSecret},
		{name: "invalid access ttl", cfg: config.JWTConfig{Secret: testJWTConfig().Secret, AccessTTL: 0, RefreshTTL: time.Hour}, want: ErrInvalidAccessTTL},
		{name: "invalid refresh ttl", cfg: config.JWTConfig{Secret: testJWTConfig().Secret, AccessTTL: time.Hour, RefreshTTL: time.Hour}, want: ErrInvalidRefreshTTL},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewManager(tc.cfg)
			if !errors.Is(err, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, err)
			}
		})
	}
}

// TestParseAndValidateTokenRequiresExpiration 验证缺少 exp 的签名令牌会被拒绝。
func TestParseAndValidateTokenRequiresExpiration(t *testing.T) {
	cfg := testJWTConfig()
	manager, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("create manager: %v", err)
	}

	token := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, tokenClaims{
		UserID:    42,
		TokenType: TokenTypeAccess,
		RegisteredClaims: jwtlib.RegisteredClaims{
			IssuedAt: jwtlib.NewNumericDate(time.Now()),
		},
	})
	signedToken, err := token.SignedString([]byte(cfg.Secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	_, err = manager.ParseAndValidateToken(signedToken, TokenTypeAccess)
	if !errors.Is(err, ErrMissingExpiration) {
		t.Fatalf("expected %v, got %v", ErrMissingExpiration, err)
	}
}

// testJWTConfig 返回测试使用的有效 JWT 配置。
func testJWTConfig() config.JWTConfig {
	return config.JWTConfig{
		Secret:     "01234567890123456789012345678901",
		AccessTTL:  15 * time.Minute,
		RefreshTTL: 168 * time.Hour,
	}
}
