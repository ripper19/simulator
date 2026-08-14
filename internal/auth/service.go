package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/ripper19/simulator/internal/persistence"
)

// ErrInvalidCredentials is returned on failed login.
var ErrInvalidCredentials = errors.New("auth: invalid credentials")

// TokenPair is an access token plus its refresh token.
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
}

// Service implements registration and login on top of the user store.
type Service struct {
	store  *persistence.Store
	tokens *Manager
}

// NewService builds an auth service.
func NewService(store *persistence.Store, tokens *Manager) *Service {
	return &Service{store: store, tokens: tokens}
}

// Register creates a new USER and returns it.
func (s *Service) Register(ctx context.Context, username, password string) (persistence.UserInfo, error) {
	return s.registerRole(ctx, username, password, RoleUser)
}

// BootstrapAdmin creates an ADMIN user, used to seed the first administrator.
func (s *Service) BootstrapAdmin(ctx context.Context, username, password string) (persistence.UserInfo, error) {
	return s.registerRole(ctx, username, password, RoleAdmin)
}

func (s *Service) registerRole(ctx context.Context, username, password, role string) (persistence.UserInfo, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return persistence.UserInfo{}, err
	}
	return s.store.CreateUser(ctx, persistence.UserInfo{
		ID:           newID(),
		Username:     username,
		PasswordHash: hash,
		Role:         role,
	})
}

// Login verifies credentials and issues tokens.
func (s *Service) Login(ctx context.Context, username, password string) (TokenPair, error) {
	u, err := s.store.GetUserByUsername(ctx, username)
	if err != nil || !VerifyPassword(u.PasswordHash, password) {
		return TokenPair{}, ErrInvalidCredentials
	}
	return s.issuePair(u.ID, u.Role)
}

// Refresh issues a new access token from a valid refresh token.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (TokenPair, error) {
	claims, err := s.tokens.Parse(refreshToken, "refresh")
	if err != nil {
		return TokenPair{}, ErrInvalidCredentials
	}
	access, err := s.tokens.IssueAccess(claims.UserID, claims.Role)
	if err != nil {
		return TokenPair{}, err
	}
	return TokenPair{AccessToken: access, RefreshToken: refreshToken, TokenType: "bearer"}, nil
}

func (s *Service) issuePair(userID, role string) (TokenPair, error) {
	access, err := s.tokens.IssueAccess(userID, role)
	if err != nil {
		return TokenPair{}, err
	}
	refresh, err := s.tokens.IssueRefresh(userID, role)
	if err != nil {
		return TokenPair{}, err
	}
	return TokenPair{AccessToken: access, RefreshToken: refresh, TokenType: "bearer"}, nil
}

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return time.Now().Format("150405.000000000")
	}
	return hex.EncodeToString(b[:])
}
