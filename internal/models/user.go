package models

import "time"

type User struct {
	ID           int64
	Email        string
	Name         string
	Timezone     string
	PasswordHash string
	CreatedAt    time.Time
}

func (u User) DisplayName() string {
	if u.Name != "" {
		return u.Name
	}
	return u.Email
}
