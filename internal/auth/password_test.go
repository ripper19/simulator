package auth

import "testing"

func TestPasswordHashVerify(t *testing.T) {
	h, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if h == "correct horse battery staple" {
		t.Fatal("password was not hashed")
	}
	if !VerifyPassword(h, "correct horse battery staple") {
		t.Fatal("correct password failed verification")
	}
	if VerifyPassword(h, "wrong") {
		t.Fatal("wrong password passed verification")
	}
}

func TestPasswordHashUniqueSalt(t *testing.T) {
	a, _ := HashPassword("same")
	b, _ := HashPassword("same")
	if a == b {
		t.Fatal("two hashes of the same password should differ (random salt)")
	}
}
