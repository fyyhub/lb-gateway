package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Claims struct {
	Subject  string `json:"sub"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Expires  int64  `json:"exp"`
}

type TokenManager struct {
	secret []byte
	ttl    time.Duration
}

func NewTokenManager(secret string, ttl time.Duration) (*TokenManager, error) {
	secretBytes := []byte(secret)
	if len(secretBytes) == 0 {
		secretBytes = make([]byte, 32)
		if _, err := rand.Read(secretBytes); err != nil {
			return nil, fmt.Errorf("generate token secret: %w", err)
		}
	}
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	return &TokenManager{secret: secretBytes, ttl: ttl}, nil
}

func (m *TokenManager) Issue(userID string, username string, role string) (string, error) {
	claims := Claims{
		Subject:  userID,
		Username: username,
		Role:     role,
		Expires:  time.Now().Add(m.ttl).Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal token claims: %w", err)
	}

	payloadText := base64.RawURLEncoding.EncodeToString(payload)
	signature := sign(payloadText, m.secret)
	return payloadText + "." + signature, nil
}

func (m *TokenManager) Verify(token string) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return Claims{}, errors.New("invalid token format")
	}

	wantSignature := sign(parts[0], m.secret)
	if !hmac.Equal([]byte(parts[1]), []byte(wantSignature)) {
		return Claims{}, errors.New("invalid token signature")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Claims{}, errors.New("invalid token payload")
	}

	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Claims{}, errors.New("invalid token claims")
	}
	if claims.Expires < time.Now().Unix() {
		return Claims{}, errors.New("token expired")
	}

	return claims, nil
}

func sign(payload string, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
