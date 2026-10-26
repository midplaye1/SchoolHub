package tests

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"libraryhub/internal/model"
	"libraryhub/internal/repository"
	"libraryhub/internal/service"
	"libraryhub/migrations"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func fixture(t *testing.T) (context.Context, *service.Service, *repository.Repository, model.Book, model.Reader) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set; PostgreSQL integration tests skipped")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	schema := fmt.Sprintf("libraryhub_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, e := admin.Exec(c, `DROP SCHEMA `+schema+` CASCADE`); e != nil {
			t.Log(e)
		}
	})
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	cfg.ConnConfig.RuntimeParams["timezone"] = "Europe/Moscow"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err = migrations.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	// Reapplying is a no-op, including seed data.
	var before, after int
	pool.QueryRow(ctx, "SELECT count(*) FROM books").Scan(&before)
	if err = migrations.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	pool.QueryRow(ctx, "SELECT count(*) FROM books").Scan(&after)
	if before != 24 || after != before {
		t.Fatalf("seed not idempotent: %d/%d", before, after)
	}
	repo := repository.New(pool)
	svc := service.New(repo)
	book, err := svc.SaveBook(ctx, model.Book{Title: "Тестовая книга", Author: "Автор", Genre: "Тест", Year: 2020, TotalCopies: 1})
	if err != nil {
		t.Fatal(err)
	}
	reader, err := svc.SaveReader(ctx, model.Reader{Name: "Тестовый Читатель", Class: "9А"})
	if err != nil {
		t.Fatal(err)
	}
	return ctx, svc, repo, book, reader
}
func input(b model.Book, r model.Reader) model.LoanInput {
	return model.LoanInput{BookID: b.ID, ReaderID: r.ID, IssuedAt: time.Now().Format("2006-01-02"), DueAt: time.Now().AddDate(0, 0, 14).Format("2006-01-02")}
}
func TestIssueAvailableBook(t *testing.T) {
	ctx, s, r, b, reader := fixture(t)
	l, err := s.Issue(ctx, input(b, reader))
	if err != nil || l.ID == 0 {
		t.Fatalf("issue: %+v %v", l, err)
	}
	b, err = r.Book(ctx, b.ID)
	if err != nil || b.AvailableCopies != 0 {
		t.Fatalf("stock: %+v %v", b, err)
	}
}
func TestIssueUnavailableBook(t *testing.T) {
	ctx, s, r, b, reader := fixture(t)
	if _, err := s.Issue(ctx, input(b, reader)); err != nil {
		t.Fatal(err)
	}
	_, err := s.Issue(ctx, input(b, reader))
	if !errors.Is(err, model.ErrUnavailable) {
		t.Fatalf("expected unavailable, got %v", err)
	}
	b, err = r.Book(ctx, b.ID)
	if err != nil || b.AvailableCopies != 0 {
		t.Fatalf("stock: %+v %v", b, err)
	}
}
func TestReturnBook(t *testing.T) {
	ctx, s, r, b, reader := fixture(t)
	l, err := s.Issue(ctx, input(b, reader))
	if err != nil {
		t.Fatal(err)
	}
	l, err = s.Return(ctx, l.ID, "")
	if err != nil || l.ReturnedAt == nil {
		t.Fatalf("return: %+v %v", l, err)
	}
	b, err = r.Book(ctx, b.ID)
	if err != nil || b.AvailableCopies != 1 {
		t.Fatalf("stock: %+v %v", b, err)
	}
}
func TestRepeatedReturn(t *testing.T) {
	ctx, s, r, b, reader := fixture(t)
	l, err := s.Issue(ctx, input(b, reader))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Return(ctx, l.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Return(ctx, l.ID, ""); !errors.Is(err, model.ErrReturned) {
		t.Fatalf("expected returned, got %v", err)
	}
	b, err = r.Book(ctx, b.ID)
	if err != nil || b.AvailableCopies != 1 {
		t.Fatalf("stock: %+v %v", b, err)
	}
}
func TestDeleteBookWithActiveLoan(t *testing.T) {
	ctx, s, r, b, reader := fixture(t)
	l, err := s.Issue(ctx, input(b, reader))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Delete(ctx, "books", b.ID); !errors.Is(err, model.ErrActive) {
		t.Fatalf("expected active conflict, got %v", err)
	}
	if err = s.Delete(ctx, "readers", reader.ID); !errors.Is(err, model.ErrActive) {
		t.Fatalf("expected active reader conflict, got %v", err)
	}
	if _, err = s.Return(ctx, l.ID, ""); err != nil {
		t.Fatal(err)
	}
	if err = s.Delete(ctx, "books", b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = r.Book(ctx, b.ID); !errors.Is(err, model.ErrNotFound) {
		t.Fatal("deleted book is visible")
	}
	p, err := r.Loans(ctx, model.Filter{ReaderID: reader.ID, Page: 1, Limit: 10})
	if err != nil || len(p.Items) != 1 || p.Items[0].BookTitle != b.Title {
		t.Fatalf("history lost: %+v %v", p, err)
	}
}
func TestConcurrentLastCopyAndReturn(t *testing.T) {
	ctx, s, r, b, reader := fixture(t)
	var issued, conflicts atomic.Int32
	var loanID atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			l, err := s.Issue(ctx, input(b, reader))
			if err == nil {
				issued.Add(1)
				loanID.Store(l.ID)
			} else if errors.Is(err, model.ErrUnavailable) {
				conflicts.Add(1)
			} else {
				t.Errorf("issue: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()
	if issued.Load() != 1 || conflicts.Load() != 9 {
		t.Fatalf("issued=%d conflicts=%d", issued.Load(), conflicts.Load())
	}
	var returned atomic.Int32
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Return(ctx, loanID.Load(), "")
			if err == nil {
				returned.Add(1)
			} else if !errors.Is(err, model.ErrReturned) {
				t.Errorf("return: %v", err)
			}
		}()
	}
	wg.Wait()
	b, err := r.Book(ctx, b.ID)
	if err != nil || returned.Load() != 1 || b.AvailableCopies != 1 {
		t.Fatalf("returned=%d stock=%d err=%v", returned.Load(), b.AvailableCopies, err)
	}
}
