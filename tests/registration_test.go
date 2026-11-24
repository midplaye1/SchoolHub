package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"libraryhub/internal/auth"
	"libraryhub/internal/handler"
)

func TestRegistrationAndRoles(t *testing.T) {
	ctx, svc, repo, _, _ := fixture(t)
	t.Setenv("ADMIN_LOGIN", "admin")
	t.Setenv("ADMIN_PASSWORD", "long-admin-password")
	if e := (&auth.Auth{DB: repo.DB}).Bootstrap(ctx); e != nil {
		t.Fatal(e)
	}
	router := handler.Router(repo, svc, "../web", 10*time.Second)
	req := func(method, path, data string, c *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(data))
		r.Header.Set("Content-Type", "application/json")
		if c != nil {
			r.AddCookie(c)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	expect := func(w *httptest.ResponseRecorder, code int) {
		t.Helper()
		if w.Code != code {
			t.Fatalf("got %d want %d: %s", w.Code, code, w.Body.String())
		}
	}
	login := func(name, password string) *http.Cookie {
		w := req("POST", "/api/auth/login", fmt.Sprintf(`{"login":%q,"password":%q}`, name, password), nil)
		expect(w, 200)
		return w.Result().Cookies()[0]
	}
	payload := `{"login":" New.Student ","password":"student-password","name":" Иван Петров ","class":" 9а "}`
	w := req("POST", "/api/auth/register", payload, nil)
	expect(w, 201)
	var user auth.User
	if e := json.Unmarshal(w.Body.Bytes(), &user); e != nil {
		t.Fatal(e)
	}
	if user.Role != "reader" || user.ReaderID == nil || user.Login != "new.student" {
		t.Fatalf("invalid registration: %+v", user)
	}
	var name, class string
	if e := repo.DB.QueryRow(ctx, "SELECT name,class FROM readers WHERE id=$1", *user.ReaderID).Scan(&name, &class); e != nil || name != "Иван Петров" || class != "9А" {
		t.Fatalf("profile: %s %s %v", name, class, e)
	}
	var before, after int
	repo.DB.QueryRow(ctx, "SELECT count(*) FROM readers").Scan(&before)
	expect(req("POST", "/api/auth/register", payload, nil), 409)
	repo.DB.QueryRow(ctx, "SELECT count(*) FROM readers").Scan(&after)
	if before != after {
		t.Fatal("orphan reader on duplicate login")
	}
	for _, extra := range []string{`,"role":"librarian"`, `,"reader_id":1`} {
		expect(req("POST", "/api/auth/register", strings.TrimSuffix(payload, "}")+extra+"}", nil), 400)
	}
	expect(req("POST", "/api/auth/register", `{"login":"bad","password":"short","name":"A","class":""}`, nil), 400)
	expect(req("POST", "/api/auth/register", payload+` {}`, nil), 400)
	student := login("new.student", "student-password")
	admin := login("admin", "long-admin-password")
	path := fmt.Sprintf("/api/accounts/%d/role", user.ID)
	expect(req("GET", "/api/accounts", "", student), 403)
	expect(req("PATCH", path, `{"role":"librarian"}`, student), 403)
	expect(req("PATCH", path, `{"role":"librarian"}`, nil), 401)
	w = req("GET", "/api/accounts?search=new.student", "", admin)
	expect(w, 200)
	if strings.Contains(w.Body.String(), "password") {
		t.Fatal("password leaked")
	}
	expect(req("PATCH", path, `{"role":"owner"}`, admin), 400)
	expect(req("PATCH", path, `{"role":"librarian"}`, admin), 200)
	expect(req("GET", "/api/auth/me", "", student), 401)
	promoted := login("new.student", "student-password")
	expect(req("GET", "/api/readers", "", promoted), 200)
	var readerID int64
	repo.DB.QueryRow(ctx, "SELECT reader_id FROM accounts WHERE id=$1", user.ID).Scan(&readerID)
	if readerID != *user.ReaderID {
		t.Fatal("reader profile lost on promotion")
	}
	expect(req("PATCH", path, `{"role":"reader"}`, admin), 200)
	expect(req("GET", "/api/readers", "", promoted), 401)
	student = login("new.student", "student-password")
	expect(req("GET", "/api/readers", "", student), 403)
	var adminID int64
	repo.DB.QueryRow(ctx, "SELECT id FROM accounts WHERE login='admin'").Scan(&adminID)
	expect(req("PATCH", fmt.Sprintf("/api/accounts/%d/role", adminID), `{"role":"reader"}`, admin), 409)
	// Race on the same login: only one reader profile may be committed.
	repo.DB.QueryRow(ctx, "SELECT count(*) FROM readers").Scan(&before)
	racePayload := `{"login":"race.student","password":"student-password","name":"Другой Ученик","class":"10Б"}`
	results := make(chan int, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- req("POST", "/api/auth/register", racePayload, nil).Code }()
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for code := range results {
		if code == 201 {
			success++
		}
		if code == 409 {
			conflict++
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("registration race: %d/%d", success, conflict)
	}
	repo.DB.QueryRow(ctx, "SELECT count(*) FROM readers").Scan(&after)
	if after != before+1 {
		t.Fatal("orphan profiles after concurrent signup")
	}
}
