package domain

import "time"

type UserRole string

const (
	UserRoleUser  UserRole = "user"
	UserRoleAdmin UserRole = "admin"
)

type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	DisplayName  string    `json:"display_name"`
	PasswordHash string    `json:"-"` // never serialize
	Role         UserRole  `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type UserProgress struct {
	ProblemsAttempted int                        `json:"problems_attempted"`
	ProblemsSolved    int                        `json:"problems_solved"`
	ByCategory        map[string]CategoryProgress `json:"by_category"`
	CurrentStreak     int                        `json:"current_streak"`
	LongestStreak     int                        `json:"longest_streak"`
}

type CategoryProgress struct {
	Attempted int `json:"attempted"`
	Solved    int `json:"solved"`
}
