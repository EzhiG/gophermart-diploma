package model

import "github.com/golang-jwt/jwt/v4"

type UserID int64

type UserBalance struct {
	Current   float64 `json:"current"`
	Withdrawn float64 `json:"withdrawn"`
}

type UserCredentials struct {
	ID           UserID `json:"id"`
	Login        string `json:"login"`
	PasswordHash string `json:"-"`
}

type Claims struct {
	jwt.RegisteredClaims
	UserID UserID
}
