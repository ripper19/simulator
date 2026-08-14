package auth

import (
	"testing"
	"time"
)

func TestTokenIssueParse(t *testing.T) {
	m := NewManager("secret", time.Minute, time.Hour)
	token, err := m.IssueAccess("user-1", RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := m.Parse(token, "access")
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != "user-1" || claims.Role != RoleUser {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestTokenKindEnforced(t *testing.T) {
	m := NewManager("secret", time.Minute, time.Hour)
	refresh, _ := m.IssueRefresh("u", RoleUser)
	if _, err := m.Parse(refresh, "access"); err == nil {
		t.Fatal("refresh token must not be accepted as an access token")
	}
	access, _ := m.IssueAccess("u", RoleUser)
	if _, err := m.Parse(access, "refresh"); err == nil {
		t.Fatal("access token must not be accepted as a refresh token")
	}
}

func TestTokenWrongSecret(t *testing.T) {
	a := NewManager("secret-a", time.Minute, time.Hour)
	b := NewManager("secret-b", time.Minute, time.Hour)
	token, _ := a.IssueAccess("u", RoleUser)
	if _, err := b.Parse(token, "access"); err == nil {
		t.Fatal("token signed with a different secret should fail parsing")
	}
}

func TestTokenExpired(t *testing.T) {
	m := NewManager("secret", -time.Minute, time.Hour)
	token, _ := m.IssueAccess("u", RoleUser)
	if _, err := m.Parse(token, "access"); err == nil {
		t.Fatal("expired token should fail parsing")
	}
}
