package handler

import (
	"encoding/json"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"io"
	security "libraryhub/internal/middleware"
	"libraryhub/internal/model"
	"libraryhub/internal/repository"
	"libraryhub/internal/service"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

type Handler struct {
	Repo    *repository.Repository
	Service *service.Service
}

func Router(repo *repository.Repository, svc *service.Service, webDir string, timeout time.Duration) http.Handler {
	h := &Handler{repo, svc}
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.Logger, middleware.Recoverer, security.Security)
	r.Route("/api", func(r chi.Router) {
		r.Use(middleware.Timeout(timeout))
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Cache-Control", "no-store")
				next.ServeHTTP(w, r)
			})
		})
		r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
			if err := repo.DB.Ping(r.Context()); err != nil {
				write(w, 503, map[string]string{"error": "База данных недоступна"})
				return
			}
			write(w, 200, map[string]string{"status": "ok"})
		})
		r.Get("/books", h.books)
		r.Get("/books/{id}", h.book)
		r.Post("/books", h.saveBook)
		r.Put("/books/{id}", h.saveBook)
		r.Delete("/books/{id}", func(w http.ResponseWriter, r *http.Request) { h.delete(w, r, "books") })
		r.Get("/readers", h.readers)
		r.Get("/readers/{id}", h.reader)
		r.Post("/readers", h.saveReader)
		r.Put("/readers/{id}", h.saveReader)
		r.Delete("/readers/{id}", func(w http.ResponseWriter, r *http.Request) { h.delete(w, r, "readers") })
		r.Get("/loans", h.loans)
		r.Post("/loans", h.issue)
		r.Post("/loans/{id}/return", h.returnLoan)
		r.Get("/stats/overview", h.overview)
		for _, kind := range []string{"popular-books", "genres", "popular-genres", "loans-by-month"} {
			k := kind
			r.Get("/stats/"+k, func(w http.ResponseWriter, r *http.Request) { v, e := repo.Metrics(r.Context(), k); respond(w, v, e) })
		}
		for _, kind := range []string{"genres", "classes"} {
			k := kind
			r.Get("/options/"+k, func(w http.ResponseWriter, r *http.Request) { v, e := repo.Options(r.Context(), k); respond(w, v, e) })
		}
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			write(w, 404, map[string]string{"error": "API маршрут не найден"})
		})
		r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
			write(w, 405, map[string]string{"error": "Метод не поддерживается"})
		})
	})
	r.Handle("/*", http.FileServer(http.Dir(webDir)))
	return r
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}
func fail(w http.ResponseWriter, err error) {
	status := 500
	msg := "Внутренняя ошибка сервера. Попробуйте ещё раз."
	var validation model.ValidationError
	switch {
	case errors.As(err, &validation):
		status = 400
		msg = err.Error()
	case errors.Is(err, model.ErrNotFound):
		status = 404
		msg = err.Error()
	case errors.Is(err, model.ErrUnavailable), errors.Is(err, model.ErrReturned), errors.Is(err, model.ErrActive), errors.Is(err, model.ErrCopies):
		status = 409
		msg = err.Error()
	default:
		slog.Error("request failed", "error", err)
	}
	write(w, status, map[string]string{"error": msg})
}
func respond(w http.ResponseWriter, v any, err error) {
	if err != nil {
		fail(w, err)
		return
	}
	write(w, 200, v)
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 65536)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		fail(w, model.Invalid("Некорректный JSON или неизвестные поля"))
		return false
	}
	if err := d.Decode(new(any)); err != io.EOF {
		fail(w, model.Invalid("Ожидается один JSON-объект"))
		return false
	}
	return true
}
func id(w http.ResponseWriter, r *http.Request) (int64, bool) {
	v, e := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if e != nil || v <= 0 {
		fail(w, model.Invalid("Некорректный ID"))
		return 0, false
	}
	return v, true
}
func filter(r *http.Request) (model.Filter, error) {
	q := r.URL.Query()
	f := model.Filter{Search: q.Get("search"), Author: q.Get("author"), Genre: q.Get("genre"), Availability: q.Get("availability"), Class: q.Get("class"), Status: q.Get("status"), Sort: q.Get("sort"), Order: q.Get("order"), Page: 1, Limit: 10}
	var e error
	if q.Has("page") {
		f.Page, e = strconv.Atoi(q.Get("page"))
		if e != nil || f.Page < 1 || f.Page > 1000000 {
			return f, model.Invalid("Некорректная страница")
		}
	}
	if q.Has("limit") {
		f.Limit, e = strconv.Atoi(q.Get("limit"))
		if e != nil || f.Limit < 1 || f.Limit > 100 {
			return f, model.Invalid("Размер страницы должен быть от 1 до 100")
		}
	}
	if q.Has("reader_id") {
		f.ReaderID, e = strconv.ParseInt(q.Get("reader_id"), 10, 64)
		if e != nil || f.ReaderID < 1 {
			return f, model.Invalid("Некорректный ID читателя")
		}
	}
	if f.Availability != "" && f.Availability != "available" && f.Availability != "unavailable" {
		return f, model.Invalid("Некорректный фильтр доступности")
	}
	if f.Status != "" && f.Status != "active" && f.Status != "overdue" && f.Status != "history" {
		return f, model.Invalid("Некорректный статус выдачи")
	}
	if f.Sort != "" && f.Sort != "title" && f.Sort != "author" && f.Sort != "year" {
		return f, model.Invalid("Некорректная сортировка")
	}
	if f.Order != "" && f.Order != "asc" && f.Order != "desc" {
		return f, model.Invalid("Некорректное направление сортировки")
	}
	return f, nil
}
func (h *Handler) books(w http.ResponseWriter, r *http.Request) {
	f, e := filter(r)
	if e != nil {
		fail(w, e)
		return
	}
	v, e := h.Repo.Books(r.Context(), f)
	respond(w, v, e)
}
func (h *Handler) book(w http.ResponseWriter, r *http.Request) {
	n, ok := id(w, r)
	if !ok {
		return
	}
	v, e := h.Repo.Book(r.Context(), n)
	respond(w, v, e)
}
func (h *Handler) readers(w http.ResponseWriter, r *http.Request) {
	f, e := filter(r)
	if e != nil {
		fail(w, e)
		return
	}
	v, e := h.Repo.Readers(r.Context(), f)
	respond(w, v, e)
}
func (h *Handler) reader(w http.ResponseWriter, r *http.Request) {
	n, ok := id(w, r)
	if !ok {
		return
	}
	v, e := h.Repo.Reader(r.Context(), n)
	respond(w, v, e)
}
func (h *Handler) loans(w http.ResponseWriter, r *http.Request) {
	f, e := filter(r)
	if e != nil {
		fail(w, e)
		return
	}
	v, e := h.Repo.Loans(r.Context(), f)
	respond(w, v, e)
}
func (h *Handler) saveBook(w http.ResponseWriter, r *http.Request) {
	// Only editable fields are accepted; IDs and stock are server-owned.
	var in struct {
		Title       string `json:"title"`
		Author      string `json:"author"`
		Genre       string `json:"genre"`
		Year        int    `json:"year"`
		ISBN        string `json:"isbn"`
		Description string `json:"description"`
		Total       int    `json:"total_copies"`
	}
	if !decode(w, r, &in) {
		return
	}
	b := model.Book{Title: in.Title, Author: in.Author, Genre: in.Genre, Year: in.Year, ISBN: in.ISBN, Description: in.Description, TotalCopies: in.Total}
	status := 201
	if r.Method == http.MethodPut {
		n, ok := id(w, r)
		if !ok {
			return
		}
		b.ID = n
		status = 200
	}
	v, e := h.Service.SaveBook(r.Context(), b)
	if e != nil {
		fail(w, e)
		return
	}
	write(w, status, v)
}
func (h *Handler) saveReader(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name  string `json:"name"`
		Class string `json:"class"`
	}
	if !decode(w, r, &in) {
		return
	}
	b := model.Reader{Name: in.Name, Class: in.Class}
	status := 201
	if r.Method == http.MethodPut {
		n, ok := id(w, r)
		if !ok {
			return
		}
		b.ID = n
		status = 200
	}
	v, e := h.Service.SaveReader(r.Context(), b)
	if e != nil {
		fail(w, e)
		return
	}
	write(w, status, v)
}
func (h *Handler) delete(w http.ResponseWriter, r *http.Request, kind string) {
	n, ok := id(w, r)
	if !ok {
		return
	}
	if e := h.Service.Delete(r.Context(), kind, n); e != nil {
		fail(w, e)
		return
	}
	write(w, 204, nil)
}
func (h *Handler) issue(w http.ResponseWriter, r *http.Request) {
	var in model.LoanInput
	if !decode(w, r, &in) {
		return
	}
	v, e := h.Service.Issue(r.Context(), in)
	if e != nil {
		fail(w, e)
		return
	}
	write(w, 201, v)
}
func (h *Handler) returnLoan(w http.ResponseWriter, r *http.Request) {
	n, ok := id(w, r)
	if !ok {
		return
	}
	var in struct {
		ReturnedAt string `json:"returned_at"`
	}
	if !decode(w, r, &in) {
		return
	}
	v, e := h.Service.Return(r.Context(), n, in.ReturnedAt)
	respond(w, v, e)
}
func (h *Handler) overview(w http.ResponseWriter, r *http.Request) {
	v, e := h.Repo.Overview(r.Context())
	respond(w, v, e)
}
