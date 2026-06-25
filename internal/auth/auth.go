package auth

import (
	"fmt"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func HashPassword(password string) (string, error) {
	hashedPassword, err := argon2id.CreateHash(password, argon2id.DefaultParams)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return hashedPassword, nil
}

func CheckPasswordHash(password, hash string) (bool, error) {
	match, err := argon2id.ComparePasswordAndHash(password, hash)
	if err != nil {
		return false, fmt.Errorf("failed to check password: %w", err)
	}
	return match, nil
}

func MakeJWT(userID uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error) {
	currentTime := time.Now()
	token := jwt.NewWithClaims(
		jwt.SigningMethodHS256,
		jwt.RegisteredClaims{
			Issuer:    "chirpy-access",
			IssuedAt:  jwt.NewNumericDate(currentTime),
			ExpiresAt: jwt.NewNumericDate(currentTime.Add(expiresIn)),
			Subject:   userID.String(),
		},
	)

	tokenString, err := token.SignedString([]byte(tokenSecret))
	if err != nil {
		return "", fmt.Errorf("failed to create jwt: %w", err)
	}
	return tokenString, nil
}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {
	token, err := jwt.ParseWithClaims(tokenString, &jwt.RegisteredClaims{}, func (token *jwt.Token) (any, error) {
		return []byte(tokenSecret), nil
	}) 

	if err != nil {
		return uuid.UUID{}, fmt.Errorf("failed to validate jwt: %w", err)
	}

	userId, err := token.Claims.GetSubject()
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("failed to parse jwt claims: %w", err)
	}

	userUUID, err := uuid.Parse(userId)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("failed to extract user uuid: %w", err)
	}

	return userUUID, nil
}