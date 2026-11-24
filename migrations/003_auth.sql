CREATE TABLE accounts (
 id BIGSERIAL PRIMARY KEY,
 login TEXT NOT NULL UNIQUE CHECK(length(login) BETWEEN 3 AND 64),
 password_hash TEXT NOT NULL,
 role TEXT NOT NULL CHECK(role IN ('reader','librarian')),
 reader_id BIGINT UNIQUE REFERENCES readers(id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK ((role='reader' AND reader_id IS NOT NULL) OR (role='librarian' AND reader_id IS NULL))
);
CREATE TABLE sessions (
 token_hash TEXT PRIMARY KEY,
 account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX sessions_account ON sessions(account_id);
CREATE INDEX sessions_expiry ON sessions(expires_at);
-- Database-backed throttling persists across application restarts.
CREATE TABLE login_attempts (login TEXT PRIMARY KEY, attempts INTEGER NOT NULL, window_start TIMESTAMPTZ NOT NULL);
