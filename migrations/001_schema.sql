CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE TABLE books (
 id BIGSERIAL PRIMARY KEY,
 title TEXT NOT NULL CHECK (length(trim(title)) BETWEEN 1 AND 250),
 author TEXT NOT NULL CHECK (length(trim(author)) BETWEEN 1 AND 200),
 genre TEXT NOT NULL CHECK (length(trim(genre)) BETWEEN 1 AND 100),
 year INTEGER NOT NULL CHECK (year BETWEEN 1 AND 2100),
 isbn TEXT NOT NULL DEFAULT '' CHECK (length(isbn)<=32),
 description TEXT NOT NULL DEFAULT '' CHECK (length(description)<=5000),
 total_copies INTEGER NOT NULL CHECK (total_copies BETWEEN 0 AND 100000),
 available_copies INTEGER NOT NULL CHECK (available_copies BETWEEN 0 AND total_copies),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE readers (
 id BIGSERIAL PRIMARY KEY,
 name TEXT NOT NULL CHECK (length(trim(name)) BETWEEN 2 AND 200),
 class TEXT NOT NULL CHECK (length(trim(class)) BETWEEN 1 AND 20),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE loans (
 id BIGSERIAL PRIMARY KEY,
 book_id BIGINT NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
 reader_id BIGINT NOT NULL REFERENCES readers(id) ON DELETE RESTRICT,
 issued_at DATE NOT NULL,
 due_at DATE NOT NULL CHECK (due_at >= issued_at),
 returned_at DATE CHECK (returned_at >= issued_at),
 -- Snapshots preserve history when an inactive book or reader is deleted.
 book_title TEXT NOT NULL,
 book_author TEXT NOT NULL,
 reader_name TEXT NOT NULL,
 reader_class TEXT NOT NULL
);
-- Soft deletion lets historical loans keep valid foreign keys.
ALTER TABLE books ADD COLUMN deleted_at TIMESTAMPTZ;
ALTER TABLE readers ADD COLUMN deleted_at TIMESTAMPTZ;
CREATE INDEX books_title_search ON books USING gin(title gin_trgm_ops);
CREATE INDEX books_author_search ON books USING gin(author gin_trgm_ops);
CREATE INDEX books_genre ON books(genre) WHERE deleted_at IS NULL;
CREATE INDEX readers_name_search ON readers USING gin(name gin_trgm_ops);
CREATE INDEX readers_class ON readers(class) WHERE deleted_at IS NULL;
CREATE INDEX loans_active_book ON loans(book_id) WHERE returned_at IS NULL;
CREATE INDEX loans_active_reader ON loans(reader_id) WHERE returned_at IS NULL;
CREATE INDEX loans_overdue ON loans(due_at) WHERE returned_at IS NULL;
CREATE INDEX loans_issued ON loans(issued_at);
