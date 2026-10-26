package model

import (
	"errors"
	"time"
)

var (
	ErrNotFound    = errors.New("Запись не найдена")
	ErrUnavailable = errors.New("Нет доступных экземпляров книги")
	ErrReturned    = errors.New("Эта книга уже возвращена")
	ErrActive      = errors.New("Нельзя удалить запись с активными выдачами")
	ErrCopies      = errors.New("Количество экземпляров меньше количества книг на руках")
)

type ValidationError struct{ Message string }

func (e ValidationError) Error() string { return e.Message }
func Invalid(s string) error            { return ValidationError{s} }

type Book struct {
	ID              int64     `json:"id"`
	Title           string    `json:"title"`
	Author          string    `json:"author"`
	Genre           string    `json:"genre"`
	Year            int       `json:"year"`
	ISBN            string    `json:"isbn"`
	Description     string    `json:"description"`
	TotalCopies     int       `json:"total_copies"`
	AvailableCopies int       `json:"available_copies"`
	CreatedAt       time.Time `json:"created_at"`
}
type Reader struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Class     string    `json:"class"`
	CreatedAt time.Time `json:"created_at"`
}
type Loan struct {
	ID          int64      `json:"id"`
	BookID      int64      `json:"book_id"`
	ReaderID    int64      `json:"reader_id"`
	BookTitle   string     `json:"book_title"`
	BookAuthor  string     `json:"book_author"`
	ReaderName  string     `json:"reader_name"`
	ReaderClass string     `json:"reader_class"`
	IssuedAt    time.Time  `json:"issued_at"`
	DueAt       time.Time  `json:"due_at"`
	ReturnedAt  *time.Time `json:"returned_at"`
	Overdue     bool       `json:"overdue"`
}
type LoanInput struct {
	BookID   int64  `json:"book_id"`
	ReaderID int64  `json:"reader_id"`
	IssuedAt string `json:"issued_at"`
	DueAt    string `json:"due_at"`
}
type Filter struct {
	Search, Author, Genre, Availability, Class, Status, Sort, Order string
	Page, Limit                                                     int
	ReaderID                                                        int64
}
type Page[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
	Page  int `json:"page"`
	Limit int `json:"limit"`
}
type Overview struct {
	Books     int `json:"books"`
	Copies    int `json:"copies"`
	Available int `json:"available"`
	Readers   int `json:"readers"`
	Active    int `json:"active"`
	Overdue   int `json:"overdue"`
	ThisMonth int `json:"this_month"`
}
type Metric struct {
	Label  string `json:"label"`
	Value  int    `json:"value"`
	Author string `json:"author,omitempty"`
}
