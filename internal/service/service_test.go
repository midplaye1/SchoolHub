package service

import (
	"context"
	"errors"
	"libraryhub/internal/model"
	"testing"
	"time"
)

// A recording store isolates validation from SQL; real stock/transaction tests
// live in tests/integration_test.go and use PostgreSQL.
type recordingStore struct {
	calls    int
	book     model.Book
	reader   model.Reader
	loan     model.LoanInput
	returned string
	err      error
}

func (f *recordingStore) SaveBook(_ context.Context, b model.Book) (model.Book, error) {
	f.calls++
	f.book = b
	return b, f.err
}
func (f *recordingStore) SaveReader(_ context.Context, r model.Reader) (model.Reader, error) {
	f.calls++
	f.reader = r
	return r, f.err
}
func (f *recordingStore) Issue(_ context.Context, l model.LoanInput) (model.Loan, error) {
	f.calls++
	f.loan = l
	return model.Loan{}, f.err
}
func (f *recordingStore) Return(_ context.Context, _ int64, d string) (model.Loan, error) {
	f.calls++
	f.returned = d
	return model.Loan{}, f.err
}
func (f *recordingStore) Delete(_ context.Context, _ string, _ int64) error { f.calls++; return f.err }
func testService() (*Service, *recordingStore) {
	f := &recordingStore{}
	s := New(f)
	s.Now = func() time.Time { return time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC) }
	return s, f
}
func TestIssueValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   model.LoanInput
	}{
		{"missing book", model.LoanInput{ReaderID: 1, IssuedAt: "2026-10-26", DueAt: "2026-11-09"}},
		{"missing reader", model.LoanInput{BookID: 1, IssuedAt: "2026-10-26", DueAt: "2026-11-09"}},
		{"future issue", model.LoanInput{BookID: 1, ReaderID: 1, IssuedAt: "2026-10-27", DueAt: "2026-11-09"}},
		{"due before issue", model.LoanInput{BookID: 1, ReaderID: 1, IssuedAt: "2026-10-26", DueAt: "2026-10-25"}},
		{"invalid date", model.LoanInput{BookID: 1, ReaderID: 1, IssuedAt: "2026-02-30", DueAt: "2026-11-09"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, f := testService()
			_, err := s.Issue(context.Background(), tc.in)
			var v model.ValidationError
			if !errors.As(err, &v) || f.calls != 0 {
				t.Fatalf("invalid issue reached store: %v, calls=%d", err, f.calls)
			}
		})
	}
}
func TestIssuePassesValidDatesAndPropagatesConflict(t *testing.T) {
	s, f := testService()
	f.err = model.ErrUnavailable
	in := model.LoanInput{BookID: 2, ReaderID: 3, IssuedAt: "2026-10-26", DueAt: "2026-11-09"}
	_, err := s.Issue(context.Background(), in)
	if !errors.Is(err, model.ErrUnavailable) || f.loan != in || f.calls != 1 {
		t.Fatalf("unexpected result: %v %+v", err, f.loan)
	}
}
func TestReturnDefaultsToToday(t *testing.T) {
	s, f := testService()
	_, err := s.Return(context.Background(), 1, "")
	if err != nil || f.returned != "2026-10-26" {
		t.Fatalf("default return date: %s, %v", f.returned, err)
	}
}
func TestReturnRejectsFuture(t *testing.T) {
	s, f := testService()
	_, err := s.Return(context.Background(), 1, "2026-10-27")
	if err == nil || f.calls != 0 {
		t.Fatal("future return accepted")
	}
}
func TestBookValidationAndNormalization(t *testing.T) {
	s, f := testService()
	b := model.Book{Title: "  Книга  ", Author: "  Автор ", Genre: " Наука ", Year: 2020, TotalCopies: 1}
	if _, err := s.SaveBook(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	if f.book.Title != "Книга" || f.book.Author != "Автор" || f.book.Genre != "Наука" {
		t.Fatalf("not trimmed: %+v", f.book)
	}
	for _, change := range []func(*model.Book){func(b *model.Book) { b.Title = " " }, func(b *model.Book) { b.TotalCopies = -1 }, func(b *model.Book) { b.Year = 2028 }, func(b *model.Book) { b.Description = string(make([]byte, 5001)) }} {
		s, f = testService()
		bad := b
		change(&bad)
		if _, err := s.SaveBook(context.Background(), bad); err == nil || f.calls != 0 {
			t.Fatal("invalid book accepted")
		}
	}
}
func TestReaderValidation(t *testing.T) {
	s, f := testService()
	_, err := s.SaveReader(context.Background(), model.Reader{Name: " Иван Петров ", Class: " 9а "})
	if err != nil || f.reader.Class != "9А" || f.reader.Name != "Иван Петров" {
		t.Fatalf("normalization: %+v %v", f.reader, err)
	}
	s, f = testService()
	_, err = s.SaveReader(context.Background(), model.Reader{Name: "А", Class: ""})
	if err == nil || f.calls != 0 {
		t.Fatal("invalid reader accepted")
	}
}
func TestDeletePropagatesActiveLoanConflict(t *testing.T) {
	s, f := testService()
	f.err = model.ErrActive
	if err := s.Delete(context.Background(), "books", 1); !errors.Is(err, model.ErrActive) {
		t.Fatal(err)
	}
	if f.calls != 1 {
		t.Fatal("delete not delegated")
	}
}
