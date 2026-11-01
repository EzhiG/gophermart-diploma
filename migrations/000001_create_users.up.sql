CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    login TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    current NUMERIC(10, 2) NOT NULL DEFAULT 0 CHECK (current >= 0),
    withdrawn NUMERIC(10, 2) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)
