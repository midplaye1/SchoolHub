package auth

import (
	"net/http"
	"testing"
)

func TestPermissions(t *testing.T) {
	reader := User{Role: "reader"}
	admin := User{Role: "librarian"}
	for _, tc := range []struct {
		method, path string
		allowed      bool
	}{
		{"GET", "/api/books", true}, {"GET", "/api/books/1", true}, {"GET", "/api/loans", true}, {"GET", "/api/readers", false}, {"GET", "/api/stats/overview", false}, {"POST", "/api/accounts", false}, {"DELETE", "/api/books/1", false}, {"POST", "/api/loans/1/return", false},
	} {
		if Allowed(reader, tc.method, tc.path) != tc.allowed {
			t.Errorf("reader: %s %s", tc.method, tc.path)
		}
		if !Allowed(admin, tc.method, tc.path) {
			t.Errorf("admin: %s", tc.path)
		}
	}
	if !Allowed(reader, http.MethodPost, "/api/auth/logout") {
		t.Fatal("cannot logout")
	}
}
func TestCredentials(t *testing.T) {
	if !Valid("student.one", "long-password") {
		t.Fatal("valid rejected")
	}
	for _, in := range [][2]string{{"a", "long-password"}, {"admin", "short"}, {"bad name", "long-password"}} {
		if Valid(in[0], in[1]) {
			t.Fatal("invalid accepted")
		}
	}
}
