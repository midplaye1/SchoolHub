package repository

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"libraryhub/internal/model"
	"time"
)

type Repository struct{ DB *pgxpool.Pool }

func New(db *pgxpool.Pool) *Repository { return &Repository{db} }
func normalize(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return model.ErrNotFound
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		switch pe.Code {
		case "23503":
			return model.ErrNotFound
		case "23514", "22001", "22003":
			return model.Invalid("Данные не соответствуют ограничениям базы данных")
		}
	}
	return err
}

const bookCols = `id,title,author,genre,year,isbn,description,total_copies,available_copies,created_at`

func scanBook(row pgx.Row) (b model.Book, err error) {
	err = row.Scan(&b.ID, &b.Title, &b.Author, &b.Genre, &b.Year, &b.ISBN, &b.Description, &b.TotalCopies, &b.AvailableCopies, &b.CreatedAt)
	return b, normalize(err)
}
func (r *Repository) Book(ctx context.Context, id int64) (model.Book, error) {
	return scanBook(r.DB.QueryRow(ctx, `SELECT `+bookCols+` FROM books WHERE id=$1 AND deleted_at IS NULL`, id))
}
func (r *Repository) Books(ctx context.Context, f model.Filter) (model.Page[model.Book], error) {
	p := model.Page[model.Book]{Items: []model.Book{}, Page: f.Page, Limit: f.Limit}
	where := ` FROM books WHERE deleted_at IS NULL AND ($1='' OR title ILIKE '%'||$1||'%') AND ($2='' OR author ILIKE '%'||$2||'%') AND ($3='' OR genre=$3) AND ($4='' OR ($4='available' AND available_copies>0) OR ($4='unavailable' AND available_copies=0))`
	args := []any{f.Search, f.Author, f.Genre, f.Availability}
	if err := r.DB.QueryRow(ctx, `SELECT count(*)`+where, args...).Scan(&p.Total); err != nil {
		return p, err
	}
	sort := map[string]string{"title": "title", "author": "author", "year": "year"}[f.Sort]
	if sort == "" {
		sort = "title"
	}
	order := " ASC"
	if f.Order == "desc" {
		order = " DESC"
	}
	args = append(args, f.Limit, (f.Page-1)*f.Limit)
	rows, err := r.DB.Query(ctx, `SELECT `+bookCols+where+` ORDER BY `+sort+order+`,id LIMIT $5 OFFSET $6`, args...)
	if err != nil {
		return p, err
	}
	defer rows.Close()
	for rows.Next() {
		b, e := scanBook(rows)
		if e != nil {
			return p, e
		}
		p.Items = append(p.Items, b)
	}
	return p, rows.Err()
}
func (r *Repository) SaveBook(ctx context.Context, b model.Book) (model.Book, error) {
	if b.ID == 0 {
		return scanBook(r.DB.QueryRow(ctx, `INSERT INTO books(title,author,genre,year,isbn,description,total_copies,available_copies) VALUES($1,$2,$3,$4,$5,$6,$7,$7) RETURNING `+bookCols, b.Title, b.Author, b.Genre, b.Year, b.ISBN, b.Description, b.TotalCopies))
	}
	// Lock before inspecting stock; issue/return/delete use the same row lock.
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return b, err
	}
	defer tx.Rollback(ctx)
	var total, available int
	err = tx.QueryRow(ctx, `SELECT total_copies,available_copies FROM books WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, b.ID).Scan(&total, &available)
	if err != nil {
		return b, normalize(err)
	}
	if b.TotalCopies < total-available {
		return b, model.ErrCopies
	}
	saved, err := scanBook(tx.QueryRow(ctx, `UPDATE books SET title=$2,author=$3,genre=$4,year=$5,isbn=$6,description=$7,total_copies=$8,available_copies=$9 WHERE id=$1 RETURNING `+bookCols, b.ID, b.Title, b.Author, b.Genre, b.Year, b.ISBN, b.Description, b.TotalCopies, b.TotalCopies-(total-available)))
	if err != nil {
		return b, err
	}
	return saved, tx.Commit(ctx)
}
func (r *Repository) Reader(ctx context.Context, id int64) (b model.Reader, err error) {
	err = r.DB.QueryRow(ctx, `SELECT id,name,class,created_at FROM readers WHERE id=$1 AND deleted_at IS NULL`, id).Scan(&b.ID, &b.Name, &b.Class, &b.CreatedAt)
	return b, normalize(err)
}
func (r *Repository) Readers(ctx context.Context, f model.Filter) (model.Page[model.Reader], error) {
	p := model.Page[model.Reader]{Items: []model.Reader{}, Page: f.Page, Limit: f.Limit}
	where := ` FROM readers WHERE deleted_at IS NULL AND ($1='' OR name ILIKE '%'||$1||'%') AND ($2='' OR class=$2)`
	if err := r.DB.QueryRow(ctx, `SELECT count(*)`+where, f.Search, f.Class).Scan(&p.Total); err != nil {
		return p, err
	}
	rows, err := r.DB.Query(ctx, `SELECT id,name,class,created_at`+where+` ORDER BY name,id LIMIT $3 OFFSET $4`, f.Search, f.Class, f.Limit, (f.Page-1)*f.Limit)
	if err != nil {
		return p, err
	}
	defer rows.Close()
	for rows.Next() {
		var b model.Reader
		if err = rows.Scan(&b.ID, &b.Name, &b.Class, &b.CreatedAt); err != nil {
			return p, err
		}
		p.Items = append(p.Items, b)
	}
	return p, rows.Err()
}
func (r *Repository) SaveReader(ctx context.Context, b model.Reader) (model.Reader, error) {
	var err error
	if b.ID == 0 {
		err = r.DB.QueryRow(ctx, `INSERT INTO readers(name,class) VALUES($1,$2) RETURNING id,created_at`, b.Name, b.Class).Scan(&b.ID, &b.CreatedAt)
	} else {
		err = r.DB.QueryRow(ctx, `UPDATE readers SET name=$2,class=$3 WHERE id=$1 AND deleted_at IS NULL RETURNING created_at`, b.ID, b.Name, b.Class).Scan(&b.CreatedAt)
	}
	return b, normalize(err)
}
func (r *Repository) Delete(ctx context.Context, kind string, id int64) error {
	table, col := "books", "book_id"
	if kind == "readers" {
		table, col = "readers", "reader_id"
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var locked int64
	if err = tx.QueryRow(ctx, `SELECT id FROM `+table+` WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, id).Scan(&locked); err != nil {
		return normalize(err)
	}
	var active bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM loans WHERE `+col+`=$1 AND returned_at IS NULL)`, id).Scan(&active); err != nil {
		return err
	}
	if active {
		return model.ErrActive
	}
	if _, err = tx.Exec(ctx, `UPDATE `+table+` SET deleted_at=now() WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

const loanCols = `id,book_id,reader_id,book_title,book_author,reader_name,reader_class,issued_at,due_at,returned_at,(returned_at IS NULL AND due_at<current_date)`

func scanLoan(row pgx.Row) (b model.Loan, err error) {
	err = row.Scan(&b.ID, &b.BookID, &b.ReaderID, &b.BookTitle, &b.BookAuthor, &b.ReaderName, &b.ReaderClass, &b.IssuedAt, &b.DueAt, &b.ReturnedAt, &b.Overdue)
	return b, normalize(err)
}
func (r *Repository) Loans(ctx context.Context, f model.Filter) (model.Page[model.Loan], error) {
	p := model.Page[model.Loan]{Items: []model.Loan{}, Page: f.Page, Limit: f.Limit}
	where := ` FROM loans WHERE ($1='' OR ($1='active' AND returned_at IS NULL) OR ($1='overdue' AND returned_at IS NULL AND due_at<current_date) OR ($1='history' AND returned_at IS NOT NULL)) AND ($2::bigint=0 OR reader_id=$2) AND ($3='' OR book_title ILIKE '%'||$3||'%' OR reader_name ILIKE '%'||$3||'%')`
	if err := r.DB.QueryRow(ctx, `SELECT count(*)`+where, f.Status, f.ReaderID, f.Search).Scan(&p.Total); err != nil {
		return p, err
	}
	rows, err := r.DB.Query(ctx, `SELECT `+loanCols+where+` ORDER BY issued_at DESC,id DESC LIMIT $4 OFFSET $5`, f.Status, f.ReaderID, f.Search, f.Limit, (f.Page-1)*f.Limit)
	if err != nil {
		return p, err
	}
	defer rows.Close()
	for rows.Next() {
		b, e := scanLoan(rows)
		if e != nil {
			return p, e
		}
		p.Items = append(p.Items, b)
	}
	return p, rows.Err()
}
func (r *Repository) Issue(ctx context.Context, in model.LoanInput) (model.Loan, error) {
	var result model.Loan
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	var b model.Book
	err = tx.QueryRow(ctx, `SELECT id,title,author,available_copies FROM books WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, in.BookID).Scan(&b.ID, &b.Title, &b.Author, &b.AvailableCopies)
	if err != nil {
		return result, normalize(err)
	}
	if b.AvailableCopies == 0 {
		return result, model.ErrUnavailable
	}
	var name, class string
	err = tx.QueryRow(ctx, `SELECT name,class FROM readers WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, in.ReaderID).Scan(&name, &class)
	if err != nil {
		return result, normalize(err)
	}
	result, err = scanLoan(tx.QueryRow(ctx, `INSERT INTO loans(book_id,reader_id,issued_at,due_at,book_title,book_author,reader_name,reader_class) VALUES($1,$2,$3::date,$4::date,$5,$6,$7,$8) RETURNING `+loanCols, in.BookID, in.ReaderID, in.IssuedAt, in.DueAt, b.Title, b.Author, name, class))
	if err != nil {
		return result, err
	}
	if _, err = tx.Exec(ctx, `UPDATE books SET available_copies=available_copies-1 WHERE id=$1`, in.BookID); err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}
func (r *Repository) Return(ctx context.Context, id int64, date string) (model.Loan, error) {
	var result model.Loan
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	// Find the book, then lock book before loan: consistent with issuing/deleting.
	var bookID int64
	if err = tx.QueryRow(ctx, `SELECT book_id FROM loans WHERE id=$1`, id).Scan(&bookID); err != nil {
		return result, normalize(err)
	}
	if _, err = tx.Exec(ctx, `SELECT id FROM books WHERE id=$1 FOR UPDATE`, bookID); err != nil {
		return result, err
	}
	result, err = scanLoan(tx.QueryRow(ctx, `SELECT `+loanCols+` FROM loans WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		return result, err
	}
	if result.ReturnedAt != nil {
		return result, model.ErrReturned
	}
	day, _ := time.Parse("2006-01-02", date)
	if day.Before(result.IssuedAt) {
		return result, model.Invalid("Дата возврата раньше даты выдачи")
	}
	result, err = scanLoan(tx.QueryRow(ctx, `UPDATE loans SET returned_at=$2::date WHERE id=$1 RETURNING `+loanCols, id, date))
	if err != nil {
		return result, err
	}
	if _, err = tx.Exec(ctx, `UPDATE books SET available_copies=available_copies+1 WHERE id=$1`, bookID); err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}
func (r *Repository) Overview(ctx context.Context) (s model.Overview, err error) {
	err = r.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM books WHERE deleted_at IS NULL),(SELECT coalesce(sum(total_copies),0) FROM books WHERE deleted_at IS NULL),(SELECT coalesce(sum(available_copies),0) FROM books WHERE deleted_at IS NULL),(SELECT count(*) FROM readers WHERE deleted_at IS NULL),(SELECT count(*) FROM loans WHERE returned_at IS NULL),(SELECT count(*) FROM loans WHERE returned_at IS NULL AND due_at<current_date),(SELECT count(*) FROM loans WHERE issued_at>=date_trunc('month',current_date) AND issued_at<date_trunc('month',current_date)+interval '1 month')`).Scan(&s.Books, &s.Copies, &s.Available, &s.Readers, &s.Active, &s.Overdue, &s.ThisMonth)
	return
}
func (r *Repository) Metrics(ctx context.Context, kind string) ([]model.Metric, error) {
	var query string
	switch kind {
	case "popular-books":
		query = `SELECT b.title,count(l.id)::int,b.author FROM loans l JOIN books b ON b.id=l.book_id GROUP BY b.id ORDER BY count(l.id) DESC,b.title LIMIT 5`
	case "genres":
		query = `SELECT genre,count(*)::int,'' FROM books WHERE deleted_at IS NULL GROUP BY genre ORDER BY count(*) DESC,genre`
	case "popular-genres":
		query = `SELECT b.genre,count(l.id)::int,'' FROM loans l JOIN books b ON b.id=l.book_id GROUP BY b.genre ORDER BY count(l.id) DESC,b.genre`
	case "loans-by-month":
		query = `SELECT to_char(m,'YYYY-MM'),count(l.id)::int,'' FROM generate_series(date_trunc('month',current_date)-interval '5 months',date_trunc('month',current_date),interval '1 month') m LEFT JOIN loans l ON l.issued_at>=m AND l.issued_at<m+interval '1 month' GROUP BY m ORDER BY m`
	default:
		return nil, fmt.Errorf("unknown metric")
	}
	rows, err := r.DB.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Metric{}
	for rows.Next() {
		var m model.Metric
		if err = rows.Scan(&m.Label, &m.Value, &m.Author); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func (r *Repository) Options(ctx context.Context, kind string) ([]string, error) {
	q := `SELECT DISTINCT genre FROM books WHERE deleted_at IS NULL ORDER BY genre`
	if kind == "classes" {
		q = `SELECT DISTINCT class FROM readers WHERE deleted_at IS NULL ORDER BY class`
	}
	rows, err := r.DB.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var s string
		if err = rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
