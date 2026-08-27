package user

import "time"

// UserModel 表示 users 表中的用户持久化记录。
type UserModel struct {
	ID           int64     `gorm:"column:id;primaryKey;autoIncrement"`
	Account      string    `gorm:"column:account;size:64;not null;uniqueIndex"`
	PasswordHash string    `gorm:"column:password;size:255;not null"`
	Nickname     string    `gorm:"column:nickname;size:128;not null"`
	Role         string    `gorm:"column:role;size:32;not null"`
	AvatarURL    string    `gorm:"column:avatar_url;size:255"`
	Bio          string    `gorm:"column:bio;size:255"`
	Status       int       `gorm:"column:status;default:1"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

// TableName 返回用户持久化记录对应的数据表名称。
func (UserModel) TableName() string {
	return "users"
}
