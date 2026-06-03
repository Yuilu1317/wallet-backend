package model

import "time"

type User struct {
	ID        int64
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (User) TableName() string {
	return "users"
}
