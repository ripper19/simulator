package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Roles.
const (
	RoleUser  = "USER"
	RoleAdmin = "ADMIN"
)

// Claims are the JWT claims carried by access tokens.
type Claims struct {
	UserID string `json:"uid"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// Manager issues and validates JWTs.
type Manager struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

// NewManager returns a JWT manager with the given signing secret.
func NewManager(secret string, accessTTL, refreshTTL time.Duration) *Manager {
	return &Manager{secret: []byte(secret), accessTTL: accessTTL, refreshTTL: refreshTTL}
}

// IssueAccess returns a short-lived access token.
func (m *Manager) IssueAccess(userID, role string) (string, error) {
	return m.issue(userID, role, "access", m.accessTTL)
}

// IssueRefresh returns a longer-lived refresh token.
func (m *Manager) IssueRefresh(userID, role string) (string, error) {
	return m.issue(userID, role, "refresh", m.refreshTTL)
}

func (m *Manager) issue(userID, role, kind string, ttl time.Duration) (string, error) {
	claims := Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token.Header["typ"] = kind
	return token.SignedString(m.secret)
}

// Parse validates a token of the expected kind ("access" or "refresh") and
// returns its claims.
func (m *Manager) Parse(tokenString, kind string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("auth: unexpected signing method %v", t.Header["alg"])
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, fmt.Errorf("auth: invalid token")
	}
	if k, _ := token.Header["typ"].(string); k != kind {
		return nil, fmt.Errorf("auth: wrong token kind %q (want %q)", k, kind)
	}
	return claims, nil
}

type ctxKey int

const userKey ctxKey = 1

// WithClaims stores the authenticated claims in the context.
func WithClaims(ctx context.Context, c *Claims) context.Context {
	return context.WithValue(ctx, userKey, c)
}

// FromContext returns the authenticated claims, if any.
func FromContext(ctx context.Context) (*Claims, bool) {
	c, ok := ctx.Value(userKey).(*Claims)
	return c, ok
}

// RequireAuth returns middleware that requires a valid Bearer access token,
// injecting the claims into the request context.
func (m *Manager) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearer(r)
		if token == "" {
			writeUnauthorized(w, "missing bearer token")
			return
		}
		claims, err := m.Parse(token, "access")
		if err != nil {
			writeUnauthorized(w, "invalid token")
			return
		}
		next.ServeHTTP(w, r.WithContext(WithClaims(r.Context(), claims)))
	})
}

// RequireRole returns middleware that requires a specific role.
func (m *Manager) RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := FromContext(r.Context())
			if !ok || claims.Role != role {
				writeForbidden(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	return ""
}

func writeUnauthorized(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":{"code":"unauthorized","message":"` + msg + `"}}`))
}

func writeForbidden(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(`{"error":{"code":"forbidden","message":"insufficient role"}}`))
}
