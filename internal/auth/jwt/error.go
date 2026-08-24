package jwt

import "errors"

var (
	ErrEmptyJWTSecret     = errors.New("jwt secret is required")
	ErrWeakJWTSecret      = errors.New("jwt secret is too weak")
	ErrInvalidAccessTTL   = errors.New("jwt access ttl must be positive")
	ErrInvalidRefreshTTL  = errors.New("jwt refresh ttl must be greater than access ttl")
	ErrEmptyToken         = errors.New("token is empty")
	ErrParseJWTToken      = errors.New("parse jwt token failed")
	ErrMissingExpiration  = errors.New("token expiration is required")
	ErrInvalidTokenType   = errors.New("token type invalid")
	ErrInvalidTokenUserID = errors.New("token user id invalid")
	ErrInvalidUserID      = errors.New("user id must be positive")
	ErrGenerateTokenJTI   = errors.New("generate token jti failed")
	ErrInvalidJTISize     = errors.New("token jti size must be positive")
	ErrSignJWTToken       = errors.New("sign jwt token failed")
	ErrInvalidTTL         = errors.New("ttl must be positive")
	ErrNilManager         = errors.New("jwt manager is nil")
)
