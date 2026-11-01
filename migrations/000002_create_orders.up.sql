CREATE TYPE order_status AS ENUM ('NEW', 'PROCESSING', 'INVALID', 'PROCESSED');

CREATE TABLE orders (
    number TEXT PRIMARY KEY,
    user_id BIGINT NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id),
    status order_status NOT NULL DEFAULT 'NEW',
    accrual NUMERIC(10, 2),
    uploaded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX user_uploaded_idx ON orders(user_id, uploaded_at DESC);
CREATE INDEX unfinished_uploaded_idx ON orders(uploaded_at) WHERE status IN ('NEW', 'PROCESSING');
