package jwt

import jwtlib "github.com/golang-jwt/jwt/v4"

type Claims struct {
	UserID    int64  `json:"uid"`
	Role      string `json:"role"`
	TokenType string `json:"token_type"`
	JWTID     string `json:"jti"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

type tokenClaims struct {
	UserID    int64  `json:"uid"`
	Role      string `json:"role"`
	TokenType string `json:"token_type"`
	jwtlib.RegisteredClaims
}
