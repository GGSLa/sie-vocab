package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// GenerateToken 生成 JWT token（10 年有效期，实现"永久登录"）
func GenerateToken(userID int, secret string) (string, error) {
	claims := jwt.MapClaims{
		"user_id": float64(userID),
		"iat":     time.Now().Unix(),
		"exp":     time.Now().Add(10 * 365 * 24 * time.Hour).Unix(), // 10 年
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ValidateToken 验证 JWT token，返回 userID
func ValidateToken(tokenString string, secret string) (int, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return 0, err
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return 0, fmt.Errorf("invalid token")
	}
	userIDFloat, ok := claims["user_id"].(float64)
	if !ok {
		return 0, fmt.Errorf("invalid user_id in token")
	}
	return int(userIDFloat), nil
}
