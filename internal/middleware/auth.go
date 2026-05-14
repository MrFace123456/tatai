package middleware

import (
	"context"
	"net/http"
	"strings"
	"tatai/internal/auth"
)

type contextKey string

const (
	UserContextKey contextKey = "user"
)

// AuthMiddleware 认证中间件
func AuthMiddleware(jwtManager *auth.JWTManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 从Header获取Token
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, `{"error":"未提供认证令牌"}`, http.StatusUnauthorized)
				return
			}

			// 检查Bearer前缀
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				http.Error(w, `{"error":"无效的认证格式"}`, http.StatusUnauthorized)
				return
			}

			tokenString := parts[1]

			// 验证Token
			claims, err := jwtManager.Verify(tokenString)
			if err != nil {
				if err == auth.ErrExpiredToken {
					http.Error(w, `{"error":"认证令牌已过期"}`, http.StatusUnauthorized)
				} else {
					http.Error(w, `{"error":"无效的认证令牌"}`, http.StatusUnauthorized)
				}
				return
			}

			// 将用户信息存入上下文
			ctx := context.WithValue(r.Context(), UserContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetUserFromContext 从上下文获取用户信息
func GetUserFromContext(ctx context.Context) *auth.Claims {
	user, ok := ctx.Value(UserContextKey).(*auth.Claims)
	if !ok {
		return nil
	}
	return user
}

// OptionalAuthMiddleware 可选认证中间件（不强制要求登录）
func OptionalAuthMiddleware(jwtManager *auth.JWTManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader != "" {
				parts := strings.SplitN(authHeader, " ", 2)
				if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
					claims, err := jwtManager.Verify(parts[1])
					if err == nil {
						ctx := context.WithValue(r.Context(), UserContextKey, claims)
						r = r.WithContext(ctx)
					}
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
