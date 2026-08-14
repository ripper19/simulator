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
	claims, err := m.Parse(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != "user-1" || claims.Role != RoleUser {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestTokenWrongSecret(t *testing.T) {
	a := NewManager("secret-a", time.Minute, time.Hour)
	b := NewManager("secret-b", time.Minute, time.Hour)
	token, _ := a.IssueAccess("u", RoleUser)
	if _, err := b.Parse(token); err == nil {
		t.Fatal("token signed with a different secret should fail parsing")
	}
}

func TestTokenExpired(t *testing.T) {
	m := NewManager("secret", -time.Minute, time.Hour)
	token, _ := m.IssueAccess("u", RoleUser)
	if _, err := m.Parse(token); err == nil {
		t.Fatal("expired token should fail parsing")
	}
}
