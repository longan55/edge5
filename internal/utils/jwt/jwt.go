package jwt

import (
	"edge5/config"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrTokenExpired = errors.New("token已过期")
	ErrTokenInvalid = errors.New("token无效")
)

type Claims struct {
	UserID   uint64 `json:"user_id"`
	Username string `json:"username"`
	RoleID   uint64 `json:"role_id"`
	RoleCode string `json:"role_code"`
	jwt.RegisteredClaims
}

type RefreshClaims struct {
	UserID uint64 `json:"user_id"`
	jwt.RegisteredClaims
}

func GenerateToken(userID uint64, username, roleID, roleCode string) (string, error) {
	expireTime := time.Now().Add(time.Duration(config.CONFIG.JWT.Expire) * time.Hour)

	claims := &Claims{
		UserID:   userID,
		Username: username,
		RoleID:   0,
		RoleCode: roleCode,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expireTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "edge5",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.CONFIG.JWT.Secret))
}

func GenerateRefreshToken(userID uint64) (string, error) {
	expireTime := time.Now().Add(time.Duration(config.CONFIG.JWT.RefreshExpire) * time.Hour)

	claims := &RefreshClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expireTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "edge5-refresh",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.CONFIG.JWT.Secret))
}

func ParseToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(config.CONFIG.JWT.Secret), nil
	})

	if err != nil {
		return nil, ErrTokenInvalid
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrTokenInvalid
	}

	return claims, nil
}

func ParseRefreshToken(tokenString string) (*RefreshClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &RefreshClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(config.CONFIG.JWT.Secret), nil
	})

	if err != nil {
		return nil, ErrTokenInvalid
	}

	claims, ok := token.Claims.(*RefreshClaims)
	if !ok || !token.Valid {
		return nil, ErrTokenInvalid
	}

	return claims, nil
}
