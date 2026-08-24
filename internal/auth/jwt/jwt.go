package jwt

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v4"
	"github.com/inumoroha/video-feed/internal/config"
)

const (
	minimumSecretLength = 32
	TokenTypeAccess     = "access"
	TokenTypeRefresh    = "refresh"
)

// Manager 使用固定密钥签发和校验访问令牌、刷新令牌。
type Manager struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

// NewManager 根据 JWT 配置创建令牌管理器，并校验密钥强度和令牌有效期。
func NewManager(cfg config.JWTConfig) (*Manager, error) {
	secret := strings.TrimSpace(cfg.Secret)
	if secret == "" {
		return nil, ErrEmptyJWTSecret
	}
	if strings.Contains(secret, "${") || secret == "replace-with-strong-secret" || len([]byte(secret)) < minimumSecretLength {
		return nil, ErrWeakJWTSecret
	}
	if cfg.AccessTTL <= 0 {
		return nil, ErrInvalidAccessTTL
	}
	if cfg.RefreshTTL <= cfg.AccessTTL {
		return nil, ErrInvalidRefreshTTL
	}

	return &Manager{
		secret:     []byte(secret),
		accessTTL:  cfg.AccessTTL,
		refreshTTL: cfg.RefreshTTL,
	}, nil
}

// AccessTTL 返回访问令牌的有效期。
func (m *Manager) AccessTTL() time.Duration {
	return m.accessTTL
}

// RefreshTTL 返回刷新令牌的有效期。
func (m *Manager) RefreshTTL() time.Duration {
	return m.refreshTTL
}

// SignAccessToken 为指定用户签发访问令牌。
func (m *Manager) SignAccessToken(userID int64, role string) (string, error) {
	return m.signToken(userID, role, TokenTypeAccess, m.accessTTL)
}

// SignRefreshToken 为指定用户签发刷新令牌。
func (m *Manager) SignRefreshToken(userID int64, role string) (string, error) {
	return m.signToken(userID, role, TokenTypeRefresh, m.refreshTTL)
}

// ParseAndValidateToken 解析令牌并校验签名、有效期、令牌类型和用户标识。
func (m *Manager) ParseAndValidateToken(token, expectedType string) (*Claims, error) {
	if m == nil {
		return nil, ErrNilManager
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrEmptyToken
	}
	if strings.TrimSpace(expectedType) == "" {
		return nil, ErrInvalidTokenType
	}

	parsedClaims := &tokenClaims{}
	parsedToken, err := jwtlib.ParseWithClaims(
		token,
		parsedClaims,
		func(token *jwtlib.Token) (any, error) {
			return m.secret, nil
		},
		jwtlib.WithValidMethods([]string{jwtlib.SigningMethodHS256.Alg()}),
	)
	if err != nil {
		return nil, ErrParseJWTToken
	}
	if parsedToken == nil || !parsedToken.Valid {
		return nil, ErrParseJWTToken
	}
	if parsedClaims.ExpiresAt == nil {
		return nil, ErrMissingExpiration
	}

	if parsedClaims.TokenType != expectedType {
		return nil, ErrInvalidTokenType
	}
	if parsedClaims.UserID <= 0 {
		return nil, ErrInvalidTokenUserID
	}

	return &Claims{
		UserID:    parsedClaims.UserID,
		Role:      parsedClaims.Role,
		TokenType: parsedClaims.TokenType,
		JWTID:     parsedClaims.ID,
		IssuedAt:  claimTimeUnix(parsedClaims.IssuedAt),
		ExpiresAt: claimTimeUnix(parsedClaims.ExpiresAt),
	}, nil
}

// signToken 按指定类型和有效期生成并签名令牌。
func (m *Manager) signToken(userID int64, role, tokenType string, ttl time.Duration) (string, error) {
	if m == nil {
		return "", ErrNilManager
	}
	if userID <= 0 {
		return "", ErrInvalidUserID
	}
	if ttl <= 0 {
		return "", ErrInvalidTTL
	}
	if strings.TrimSpace(tokenType) == "" {
		return "", ErrInvalidTokenType
	}

	role = strings.TrimSpace(role)
	if role == "" {
		role = "user"
	}

	now := time.Now()
	jti, err := randomID(16)
	if err != nil {
		return "", ErrGenerateTokenJTI
	}
	claims := tokenClaims{
		UserID:    userID,
		Role:      role,
		TokenType: tokenType,
		RegisteredClaims: jwtlib.RegisteredClaims{
			IssuedAt:  jwtlib.NewNumericDate(now),
			ExpiresAt: jwtlib.NewNumericDate(now.Add(ttl)),
			ID:        jti,
		},
	}

	token := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims)
	signedToken, err := token.SignedString(m.secret)
	if err != nil {
		return "", ErrSignJWTToken
	}

	return signedToken, nil
}

// randomID 使用密码学安全随机数生成十六进制标识符。
func randomID(size int) (string, error) {
	if size <= 0 {
		return "", ErrInvalidJTISize
	}
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// claimTimeUnix 将 JWT 时间字段转换为 Unix 时间戳，缺失时返回零值。
func claimTimeUnix(value *jwtlib.NumericDate) int64 {
	if value == nil {
		return 0
	}
	return value.Unix()
}
