package tests

import (
	"encoding/json"
	"fmt"
	"libraryhub/internal/auth"
	"libraryhub/internal/handler"
	"libraryhub/internal/model"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAuthorizationLifecycle(t *testing.T) {
	ctx, svc, repo, book, reader := fixture(t)
	t.Setenv("ADMIN_LOGIN", "admin")
	t.Setenv("ADMIN_PASSWORD", "long-admin-password")
	a := &auth.Auth{DB: repo.DB}
	if e := a.Bootstrap(ctx); e != nil {
		t.Fatal(e)
	}
	router := handler.Router(repo, svc, "../web", 10*time.Second)
	request := func(method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	expect := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("status %d expected %d: %s", w.Code, status, w.Body.String())
		}
	}
	login := func(name, password string) *http.Cookie {
		t.Helper()
		w := request("POST", "/api/auth/login", fmt.Sprintf(`{"login":%q,"password":%q}`, name, password), nil)
		expect(w, 200)
		cs := w.Result().Cookies()
		if len(cs) == 0 || !cs[0].HttpOnly || cs[0].SameSite != http.SameSiteStrictMode {
			t.Fatal("unsafe/missing cookie")
		}
		return cs[0]
	}
	expect(request("GET", "/api/books", "", nil), 401)
	expect(request("POST", "/api/auth/login", `{"login":"admin","password":"wrong"}`, nil), 401)
	admin := login("admin", "long-admin-password")
	expect(request("POST", "/api/accounts", fmt.Sprintf(`{"login":"student","password":"student-password","role":"reader","reader_id":%d}`, reader.ID), admin), 201)
	student := login("student", "student-password")
	expect(request("GET", "/api/books", "", student), 200)
	for _, path := range []string{"/api/readers", "/api/stats/overview", "/api/options/classes"} {
		expect(request("GET", path, "", student), 403)
	}
	for _, path := range []string{"/api/books", "/api/readers", "/api/loans", "/api/accounts"} {
		expect(request("POST", path, `{}`, student), 403)
	}
	expect(request("PUT", fmt.Sprintf("/api/books/%d", book.ID), `{}`, student), 403)
	expect(request("DELETE", fmt.Sprintf("/api/books/%d", book.ID), `{}`, student), 403)
	if _, e := svc.Issue(ctx, input(book, reader)); e != nil {
		t.Fatal(e)
	}
	w := request("GET", "/api/loans?reader_id=1", "", student)
	expect(w, 200)
	var loans model.Page[model.Loan]
	if e := json.Unmarshal(w.Body.Bytes(), &loans); e != nil {
		t.Fatal(e)
	}
	if loans.Total != 1 || loans.Items[0].ReaderID != reader.ID {
		t.Fatal("reader can access another reader's loans")
	}
	expect(request("POST", fmt.Sprintf("/api/loans/%d/return", loans.Items[0].ID), `{}`, student), 403)
	expect(request("GET", "/api/readers", "", admin), 200)
	// Cross-origin JSON mutations must be rejected, including login.
	r := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "https://other.example")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, r)
	expect(w, 403)
	expect(request("POST", "/api/auth/logout", `{}`, student), 200)
	expect(request("GET", "/api/books", "", student), 401)
	student = login("student", "student-password")
	if _, e := repo.DB.Exec(ctx, "UPDATE sessions SET expires_at=now()-interval '1 minute'"); e != nil {
		t.Fatal(e)
	}
	expect(request("GET", "/api/books", "", student), 401)
	for i := 0; i < 10; i++ {
		expect(request("POST", "/api/auth/login", `{"login":"unknown","password":"bad"}`, nil), 401)
	}
	expect(request("POST", "/api/auth/login", `{"login":"unknown","password":"bad"}`, nil), 429)
}
