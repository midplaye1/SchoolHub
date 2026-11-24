package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID       int64  `json:"id"`
	Login    string `json:"login"`
	Role     string `json:"role"`
	ReaderID *int64 `json:"reader_id"`
}
type key struct{}

func Current(ctx context.Context) User { u, _ := ctx.Value(key{}).(User); return u }

type Auth struct{ DB *pgxpool.Pool }

var loginPattern = regexp.MustCompile(`^[a-z0-9_.-]{3,64}$`)

func Valid(login, password string) bool {
	return loginPattern.MatchString(login) && len(password) >= 10 && len(password) <= 72
}
func digest(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func problem(w http.ResponseWriter, status int, msg string) {
	reply(w, status, map[string]string{"error": msg})
}
func body(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(new(any)) != io.EOF {
		problem(w, 400, "Некорректные данные")
		return false
	}
	return true
}
func secure() bool { return os.Getenv("COOKIE_SECURE") == "true" }
func cookie(w http.ResponseWriter, token string, age int) {
	http.SetCookie(w, &http.Cookie{Name: "libraryhub_session", Value: token, Path: "/", HttpOnly: true, Secure: secure(), SameSite: http.SameSiteStrictMode, MaxAge: age})
}

// Bootstrap is serialized and only creates the first librarian, never resets passwords.
func (a *Auth) Bootstrap(ctx context.Context) error {
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(6843220)"); e != nil {
		return e
	}
	var exists bool
	if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM accounts WHERE role='librarian')").Scan(&exists); e != nil {
		return e
	}
	if exists {
		return tx.Commit(ctx)
	}
	login := strings.ToLower(strings.TrimSpace(os.Getenv("ADMIN_LOGIN")))
	password := os.Getenv("ADMIN_PASSWORD")
	if !Valid(login, password) {
		return errors.New("set ADMIN_LOGIN (3–64 lowercase Latin letters/digits/_.-) and ADMIN_PASSWORD (10–72 bytes) for the first librarian")
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, "INSERT INTO accounts(login,password_hash,role) VALUES($1,$2,'librarian')", login, string(hash))
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (a *Auth) Login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if !body(w, r, &in) {
		return
	}
	in.Login = strings.ToLower(strings.TrimSpace(in.Login))
	if len(in.Login) > 64 || len(in.Password) > 72 {
		problem(w, 401, "Неверный логин или пароль")
		return
	}
	var attempts int
	e := a.DB.QueryRow(r.Context(), `INSERT INTO login_attempts(login,attempts,window_start) VALUES($1,1,now()) ON CONFLICT(login) DO UPDATE SET attempts=CASE WHEN login_attempts.window_start<now()-interval '15 minutes' THEN 1 ELSE login_attempts.attempts+1 END,window_start=CASE WHEN login_attempts.window_start<now()-interval '15 minutes' THEN now() ELSE login_attempts.window_start END RETURNING attempts`, in.Login).Scan(&attempts)
	if e != nil {
		problem(w, 500, "Не удалось выполнить вход")
		return
	}
	if attempts > 10 {
		problem(w, 429, "Слишком много попыток. Повторите через 15 минут.")
		return
	}
	var u User
	var hash string
	e = a.DB.QueryRow(r.Context(), `SELECT a.id,a.login,a.role,a.reader_id,a.password_hash FROM accounts a LEFT JOIN readers r ON r.id=a.reader_id WHERE a.login=$1 AND (a.role='librarian' OR r.deleted_at IS NULL)`, in.Login).Scan(&u.ID, &u.Login, &u.Role, &u.ReaderID, &hash)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		problem(w, 500, "Не удалось выполнить вход")
		return
	}
	if e != nil {
		hash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
	}
	passwordErr := bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password))
	if e != nil || passwordErr != nil {
		problem(w, 401, "Неверный логин или пароль")
		return
	}
	raw := make([]byte, 32)
	if _, e = rand.Read(raw); e != nil {
		problem(w, 500, "Не удалось создать сессию")
		return
	}
	token := hex.EncodeToString(raw)
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		problem(w, 500, "Не удалось создать сессию")
		return
	}
	defer tx.Rollback(r.Context())
	if _, e = tx.Exec(r.Context(), "INSERT INTO sessions(token_hash,account_id,expires_at) VALUES($1,$2,$3)", digest(token), u.ID, time.Now().Add(24*time.Hour)); e != nil {
		problem(w, 500, "Не удалось создать сессию")
		return
	}
	if old, e := r.Cookie("libraryhub_session"); e == nil {
		_, _ = tx.Exec(r.Context(), "DELETE FROM sessions WHERE token_hash=$1", digest(old.Value))
	}
	_, e = tx.Exec(r.Context(), "DELETE FROM login_attempts WHERE login=$1", in.Login)
	if e != nil {
		problem(w, 500, "Не удалось создать сессию")
		return
	}
	if e = tx.Commit(r.Context()); e != nil {
		problem(w, 500, "Не удалось создать сессию")
		return
	}
	cookie(w, token, 86400)
	reply(w, 200, u)
}
func (a *Auth) Logout(w http.ResponseWriter, r *http.Request) {
	if c, e := r.Cookie("libraryhub_session"); e == nil {
		if _, e = a.DB.Exec(r.Context(), "DELETE FROM sessions WHERE token_hash=$1", digest(c.Value)); e != nil {
			problem(w, 500, "Не удалось выйти")
			return
		}
	}
	cookie(w, "", -1)
	reply(w, 200, map[string]bool{"ok": true})
}
func (a *Auth) Me(w http.ResponseWriter, r *http.Request) { reply(w, 200, Current(r.Context())) }

// Only librarians provision accounts; reader identities are bound by the server.
func (a *Auth) Create(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Login    string `json:"login"`
		Password string `json:"password"`
		Role     string `json:"role"`
		ReaderID *int64 `json:"reader_id"`
	}
	if !body(w, r, &in) {
		return
	}
	in.Login = strings.ToLower(strings.TrimSpace(in.Login))
	if !Valid(in.Login, in.Password) || (in.Role != "reader" && in.Role != "librarian") || (in.Role == "reader" && in.ReaderID == nil) || (in.Role == "librarian" && in.ReaderID != nil) {
		problem(w, 400, "Укажите логин (3–64: a-z, 0-9, _, ., -), пароль (10–72 байта), роль и читателя для читательского аккаунта")
		return
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if e != nil {
		problem(w, 500, "Не удалось создать аккаунт")
		return
	}
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		problem(w, 500, "Ошибка базы данных")
		return
	}
	defer tx.Rollback(r.Context())
	if in.ReaderID != nil {
		var id int64
		if e = tx.QueryRow(r.Context(), "SELECT id FROM readers WHERE id=$1 AND deleted_at IS NULL FOR UPDATE", *in.ReaderID).Scan(&id); e != nil {
			problem(w, 400, "Читатель не найден")
			return
		}
	}
	var u User
	e = tx.QueryRow(r.Context(), `INSERT INTO accounts(login,password_hash,role,reader_id) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING RETURNING id,login,role,reader_id`, in.Login, string(hash), in.Role, in.ReaderID).Scan(&u.ID, &u.Login, &u.Role, &u.ReaderID)
	if errors.Is(e, pgx.ErrNoRows) {
		problem(w, 409, "Логин или аккаунт читателя уже существует")
		return
	}
	if e != nil {
		problem(w, 500, "Не удалось создать аккаунт")
		return
	}
	if e = tx.Commit(r.Context()); e != nil {
		problem(w, 500, "Не удалось создать аккаунт")
		return
	}
	reply(w, 201, u)
}
func Allowed(u User, method, path string) bool {
	if u.Role == "librarian" {
		return true
	}
	if path == "/api/auth/me" || path == "/api/auth/logout" {
		return true
	}
	if method != http.MethodGet {
		return false
	}
	return path == "/api/books" || strings.HasPrefix(path, "/api/books/") || path == "/api/options/genres" || path == "/api/loans"
}
func (a *Auth) Guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// All browser mutations require JSON + same-origin; no permissive CORS.
		if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
				problem(w, 415, "Ожидается application/json")
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" {
				u, e := url.Parse(origin)
				scheme := "http"
				if r.TLS != nil || secure() {
					scheme = "https"
				}
				if e != nil || u.Host != r.Host || u.Scheme != scheme {
					problem(w, 403, "Запрос с другого сайта запрещён")
					return
				}
			}
		}
		if r.URL.Path == "/api/auth/login" || r.URL.Path == "/api/auth/register" || r.URL.Path == "/api/health" {
			next.ServeHTTP(w, r)
			return
		}
		c, e := r.Cookie("libraryhub_session")
		if e != nil {
			problem(w, 401, "Войдите в аккаунт")
			return
		}
		var u User
		e = a.DB.QueryRow(r.Context(), `SELECT a.id,a.login,a.role,a.reader_id FROM sessions s JOIN accounts a ON a.id=s.account_id LEFT JOIN readers r ON r.id=a.reader_id WHERE s.token_hash=$1 AND s.expires_at>now() AND (a.role='librarian' OR r.deleted_at IS NULL)`, digest(c.Value)).Scan(&u.ID, &u.Login, &u.Role, &u.ReaderID)
		if errors.Is(e, pgx.ErrNoRows) {
			cookie(w, "", -1)
			problem(w, 401, "Сессия истекла. Войдите снова.")
			return
		}
		if e != nil {
			problem(w, 500, "Не удалось проверить сессию")
			return
		}
		if !Allowed(u, r.Method, r.URL.Path) {
			problem(w, 403, "Недостаточно прав")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), key{}, u)))
	})
}
