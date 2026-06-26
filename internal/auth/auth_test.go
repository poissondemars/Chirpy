package auth

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestHashPassword_returnsNonEmptyHash(t *testing.T) {
	hash, err := HashPassword("mysecretpassword")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}
}

func TestHashPassword_differentPasswordsProduceDifferentHashes(t *testing.T) {
	hash1, err := HashPassword("password1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hash2, err := HashPassword("password2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash1 == hash2 {
		t.Fatal("expected different hashes for different passwords")
	}
}

func TestCheckPasswordHash_correctPasswordMatches(t *testing.T) {
	hash, err := HashPassword("correct-password")
	if err != nil {
		t.Fatalf("unexpected error hashing: %v", err)
	}
	match, err := CheckPasswordHash("correct-password", hash)
	if err != nil {
		t.Fatalf("unexpected error checking: %v", err)
	}
	if !match {
		t.Fatal("expected correct password to match")
	}
}

func TestCheckPasswordHash_wrongPasswordDoesNotMatch(t *testing.T) {
	hash, err := HashPassword("correct-password")
	if err != nil {
		t.Fatalf("unexpected error hashing: %v", err)
	}
	match, err := CheckPasswordHash("wrong-password", hash)
	if err != nil {
		t.Fatalf("unexpected error checking: %v", err)
	}
	if match {
		t.Fatal("expected wrong password not to match")
	}
}

func TestMakeJWT_andValidateJWT_roundTrip(t *testing.T) {
	userID := uuid.New()
	secret := "test-secret"

	token, err := MakeJWT(userID, secret, time.Hour)
	if err != nil {
		t.Fatalf("unexpected error creating JWT: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	got, err := ValidateJWT(token, secret)
	if err != nil {
		t.Fatalf("unexpected error validating JWT: %v", err)
	}
	if got != userID {
		t.Fatalf("expected userID %v, got %v", userID, got)
	}
}

func TestValidateJWT_expiredTokenIsRejected(t *testing.T) {
	userID := uuid.New()
	secret := "test-secret"

	token, err := MakeJWT(userID, secret, -time.Second)
	if err != nil {
		t.Fatalf("unexpected error creating JWT: %v", err)
	}

	_, err = ValidateJWT(token, secret)
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
}

func TestValidateJWT_wrongSecretIsRejected(t *testing.T) {
	userID := uuid.New()

	token, err := MakeJWT(userID, "correct-secret", time.Hour)
	if err != nil {
		t.Fatalf("unexpected error creating JWT: %v", err)
	}

	_, err = ValidateJWT(token, "wrong-secret")
	if err == nil {
		t.Fatal("expected error for wrong secret, got nil")
	}
}

func TestGetBearerToken_validHeader(t *testing.T) {
	headers := http.Header{}
	headers.Set("Authorization", "Bearer mytoken123")

	token, err := GetBearerToken(headers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "mytoken123" {
		t.Fatalf("expected 'mytoken123', got '%s'", token)
	}
}

func TestGetBearerToken_missingHeader(t *testing.T) {
	headers := http.Header{}

	_, err := GetBearerToken(headers)
	if err == nil {
		t.Fatal("expected error for missing Authorization header, got nil")
	}
}

func TestGetBearerToken_malformedHeader(t *testing.T) {
	cases := []string{
		"Bearer",
		"justonepart",
		"Bearer token extra",
	}
	for _, h := range cases {
		headers := http.Header{}
		headers.Set("Authorization", h)
		_, err := GetBearerToken(headers)
		if err == nil {
			t.Errorf("expected error for header %q, got nil", h)
		}
	}
}
