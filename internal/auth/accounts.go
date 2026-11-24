package auth

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

// Registration never accepts role or reader_id: it creates a new reader profile.
func (a *Auth) Register(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Login    string `json:"login"`
		Password string `json:"password"`
		Name     string `json:"name"`
		Class    string `json:"class"`
	}
	if !body(w, r, &in) {
		return
	}
	in.Login = strings.ToLower(strings.TrimSpace(in.Login))
	in.Name = strings.TrimSpace(in.Name)
	in.Class = strings.ToUpper(strings.TrimSpace(in.Class))
	if !Valid(in.Login, in.Password) || utf8.RuneCountInString(in.Name) < 2 || utf8.RuneCountInString(in.Name) > 200 || utf8.RuneCountInString(in.Class) < 1 || utf8.RuneCountInString(in.Class) > 20 {
		problem(w, 400, "Укажите ФИО (2–200 символов), класс (до 20 символов), логин (3–64: a-z, 0-9, _, ., -) и пароль (10–72 байта)")
		return
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if e != nil {
		problem(w, 500, "Не удалось создать аккаунт")
		return
	}
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		problem(w, 500, "Не удалось создать аккаунт")
		return
	}
	defer tx.Rollback(r.Context())
	var readerID int64
	if e = tx.QueryRow(r.Context(), "INSERT INTO readers(name,class) VALUES($1,$2) RETURNING id", in.Name, in.Class).Scan(&readerID); e != nil {
		problem(w, 500, "Не удалось создать профиль читателя")
		return
	}
	var u User
	e = tx.QueryRow(r.Context(), `INSERT INTO accounts(login,password_hash,role,reader_id) VALUES($1,$2,'reader',$3) ON CONFLICT(login) DO NOTHING RETURNING id,login,role,reader_id`, in.Login, string(hash), readerID).Scan(&u.ID, &u.Login, &u.Role, &u.ReaderID)
	if errors.Is(e, pgx.ErrNoRows) {
		problem(w, 409, "Этот логин уже занят")
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

type accountRow struct {
	User
	Name         string `json:"name"`
	Class        string `json:"class"`
	ReaderActive bool   `json:"reader_active"`
}

func (a *Auth) List(w http.ResponseWriter, r *http.Request) {
	page, limit := 1, 10
	var e error
	if s := r.URL.Query().Get("page"); s != "" {
		page, e = strconv.Atoi(s)
		if e != nil || page < 1 || page > 1000000 {
			problem(w, 400, "Некорректная страница")
			return
		}
	}
	search := r.URL.Query().Get("search")
	if len(search) > 500 {
		problem(w, 400, "Слишком длинный запрос")
		return
	}
	where := ` FROM accounts a LEFT JOIN readers r ON r.id=a.reader_id WHERE ($1='' OR a.login ILIKE '%'||$1||'%' OR r.name ILIKE '%'||$1||'%')`
	var total int
	if e = a.DB.QueryRow(r.Context(), "SELECT count(*)"+where, search).Scan(&total); e != nil {
		problem(w, 500, "Не удалось загрузить пользователей")
		return
	}
	rows, e := a.DB.Query(r.Context(), `SELECT a.id,a.login,a.role,a.reader_id,coalesce(r.name,''),coalesce(r.class,''),(r.id IS NOT NULL AND r.deleted_at IS NULL)`+where+` ORDER BY a.id LIMIT $2 OFFSET $3`, search, limit, (page-1)*limit)
	if e != nil {
		problem(w, 500, "Не удалось загрузить пользователей")
		return
	}
	defer rows.Close()
	items := []accountRow{}
	for rows.Next() {
		var u accountRow
		if e = rows.Scan(&u.ID, &u.Login, &u.Role, &u.ReaderID, &u.Name, &u.Class, &u.ReaderActive); e != nil {
			problem(w, 500, "Не удалось загрузить пользователей")
			return
		}
		items = append(items, u)
	}
	if rows.Err() != nil {
		problem(w, 500, "Не удалось загрузить пользователей")
		return
	}
	reply(w, 200, map[string]any{"items": items, "total": total, "page": page, "limit": limit})
}

func (a *Auth) ChangeRole(w http.ResponseWriter, r *http.Request) {
	id, e := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if e != nil || id < 1 {
		problem(w, 400, "Некорректный ID")
		return
	}
	var in struct {
		Role string `json:"role"`
	}
	if !body(w, r, &in) {
		return
	}
	if in.Role != "reader" && in.Role != "librarian" {
		problem(w, 400, "Неизвестная роль")
		return
	}
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		problem(w, 500, "Не удалось изменить роль")
		return
	}
	defer tx.Rollback(r.Context())
	// Serialize role edits and recheck the actor after acquiring the lock.
	if _, e = tx.Exec(r.Context(), "SELECT pg_advisory_xact_lock(6843220)"); e != nil {
		problem(w, 500, "Не удалось изменить роль")
		return
	}
	var actorRole string
	if e = tx.QueryRow(r.Context(), "SELECT role FROM accounts WHERE id=$1", Current(r.Context()).ID).Scan(&actorRole); e != nil || actorRole != "librarian" {
		problem(w, 403, "Недостаточно прав")
		return
	}
	var u User
	e = tx.QueryRow(r.Context(), "SELECT id,login,role,reader_id FROM accounts WHERE id=$1 FOR UPDATE", id).Scan(&u.ID, &u.Login, &u.Role, &u.ReaderID)
	if errors.Is(e, pgx.ErrNoRows) {
		problem(w, 404, "Пользователь не найден")
		return
	}
	if e != nil {
		problem(w, 500, "Не удалось изменить роль")
		return
	}
	if u.Role == in.Role {
		reply(w, 200, u)
		return
	}
	if in.Role == "reader" {
		var count int
		if e = tx.QueryRow(r.Context(), "SELECT count(*) FROM accounts WHERE role='librarian'").Scan(&count); e != nil {
			problem(w, 500, "Не удалось изменить роль")
			return
		}
		if count <= 1 {
			problem(w, 409, "Нельзя снять роль последнего библиотекаря")
			return
		}
		if u.ReaderID == nil {
			problem(w, 409, "У пользователя нет профиля читателя")
			return
		}
	}
	if u.ReaderID != nil {
		var profile int64
		if e = tx.QueryRow(r.Context(), "SELECT id FROM readers WHERE id=$1 AND deleted_at IS NULL FOR UPDATE", *u.ReaderID).Scan(&profile); e != nil {
			problem(w, 409, "Профиль читателя удалён")
			return
		}
	}
	if _, e = tx.Exec(r.Context(), "UPDATE accounts SET role=$2 WHERE id=$1", id, in.Role); e != nil {
		problem(w, 500, "Не удалось изменить роль")
		return
	}
	// Re-authentication is required after a privilege change on all devices.
	if _, e = tx.Exec(r.Context(), "DELETE FROM sessions WHERE account_id=$1", id); e != nil {
		problem(w, 500, "Не удалось изменить роль")
		return
	}
	if e = tx.Commit(r.Context()); e != nil {
		problem(w, 500, "Не удалось изменить роль")
		return
	}
	u.Role = in.Role
	reply(w, 200, u)
}
