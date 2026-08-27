package user

import "errors"

var ErrEmptyAccount = errors.New("account is required")
var ErrEmptyPassword = errors.New("password is required")
var ErrEmptyNickname = errors.New("nickname is required")
var ErrUserNotFound = errors.New("user not found")
var ErrInvalidUserID = errors.New("user id must be positive")
var ErrEmptyProfileUpdate = errors.New("profile update is required")
var ErrAccountAlreadyExists = errors.New("user account already exists")
var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrHashPasswordFailed = errors.New("hash password failed")