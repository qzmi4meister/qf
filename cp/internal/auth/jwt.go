package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	accessTokenTTL  = 30 * time.Minute
	refreshTokenTTL = 7 * 24 * time.Hour
	refreshCookie   = "qf_refresh"
	accessCookie    = "qf_token"
)

type Claims struct {
	UserID   string `json:"uid"`
	TenantID string `json:"tid"`
	Username string `json:"username"`
	Email    string `json:"email"` // kept for backward compat with existing tokens
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

func IssueAccessToken(secret []byte, userID, tenantID, username, email, role string) (string, error) {
	claims := Claims{
		UserID:   userID,
		TenantID: tenantID,
		Username: username,
		Email:    email,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(accessTokenTTL)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
}

type RefreshClaims struct {
	TokenVersion int32 `json:"tv"`
	jwt.RegisteredClaims
}

func IssueRefreshToken(secret []byte, userID string, tokenVersion int32) (string, error) {
	claims := RefreshClaims{
		TokenVersion: tokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(refreshTokenTTL)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
}

func ParseAccessToken(secret []byte, tokenStr string) (*Claims, error) {
	tok, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return secret, nil
	})
	if err != nil {
		return nil, err
	}
	c, ok := tok.Claims.(*Claims)
	if !ok || !tok.Valid {
		return nil, errors.New("invalid token claims")
	}
	return c, nil
}

func ParseRefreshToken(secret []byte, tokenStr string) (userID string, tokenVersion int32, err error) {
	tok, err := jwt.ParseWithClaims(tokenStr, &RefreshClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return secret, nil
	})
	if err != nil {
		return "", 0, err
	}
	rc, ok := tok.Claims.(*RefreshClaims)
	if !ok || !tok.Valid {
		return "", 0, errors.New("invalid refresh token")
	}
	return rc.Subject, rc.TokenVersion, nil
}
