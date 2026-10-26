package service

import (
	"context"
	"libraryhub/internal/model"
	"strings"
	"time"
	"unicode/utf8"
)

type Store interface {
	SaveBook(context.Context, model.Book) (model.Book, error)
	SaveReader(context.Context, model.Reader) (model.Reader, error)
	Delete(context.Context, string, int64) error
	Issue(context.Context, model.LoanInput) (model.Loan, error)
	Return(context.Context, int64, string) (model.Loan, error)
}
type Service struct {
	Store Store
	Now   func() time.Time
}

func New(s Store) *Service               { return &Service{Store: s, Now: time.Now} }
func length(s string, min, max int) bool { n := utf8.RuneCountInString(s); return n >= min && n <= max }
func (s *Service) SaveBook(ctx context.Context, b model.Book) (model.Book, error) {
	b.Title = strings.TrimSpace(b.Title)
	b.Author = strings.TrimSpace(b.Author)
	b.Genre = strings.TrimSpace(b.Genre)
	b.ISBN = strings.TrimSpace(b.ISBN)
	b.Description = strings.TrimSpace(b.Description)
	if !length(b.Title, 1, 250) || !length(b.Author, 1, 200) || !length(b.Genre, 1, 100) {
		return b, model.Invalid("Укажите название (до 250), автора (до 200) и жанр (до 100 символов)")
	}
	if b.Year < 1 || b.Year > s.Now().Year()+1 {
		return b, model.Invalid("Укажите корректный год издания")
	}
	if b.TotalCopies < 0 || b.TotalCopies > 100000 {
		return b, model.Invalid("Количество экземпляров должно быть от 0 до 100 000")
	}
	if !length(b.ISBN, 0, 32) || !length(b.Description, 0, 5000) {
		return b, model.Invalid("ISBN: до 32 символов; описание: до 5 000 символов")
	}
	return s.Store.SaveBook(ctx, b)
}
func (s *Service) SaveReader(ctx context.Context, r model.Reader) (model.Reader, error) {
	r.Name = strings.TrimSpace(r.Name)
	r.Class = strings.ToUpper(strings.TrimSpace(r.Class))
	if !length(r.Name, 2, 200) || !length(r.Class, 1, 20) {
		return r, model.Invalid("Укажите ФИО (2–200 символов) и класс (до 20 символов)")
	}
	return s.Store.SaveReader(ctx, r)
}
func (s *Service) Delete(ctx context.Context, kind string, id int64) error {
	if id <= 0 {
		return model.Invalid("Некорректный ID")
	}
	return s.Store.Delete(ctx, kind, id)
}
func (s *Service) Issue(ctx context.Context, in model.LoanInput) (model.Loan, error) {
	var l model.Loan
	issued, e := time.Parse("2006-01-02", in.IssuedAt)
	if e != nil {
		return l, model.Invalid("Некорректная дата выдачи")
	}
	due, e := time.Parse("2006-01-02", in.DueAt)
	if e != nil || due.Before(issued) {
		return l, model.Invalid("Дата возврата должна быть не раньше даты выдачи")
	}
	if in.BookID <= 0 || in.ReaderID <= 0 {
		return l, model.Invalid("Выберите книгу и читателя")
	}
	if in.IssuedAt > s.Now().Format("2006-01-02") {
		return l, model.Invalid("Дата выдачи не может быть в будущем")
	}
	return s.Store.Issue(ctx, in)
}
func (s *Service) Return(ctx context.Context, id int64, date string) (model.Loan, error) {
	if date == "" {
		date = s.Now().Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", date); err != nil || date > s.Now().Format("2006-01-02") {
		return model.Loan{}, model.Invalid("Укажите корректную дату возврата, не позднее сегодняшней")
	}
	return s.Store.Return(ctx, id, date)
}
