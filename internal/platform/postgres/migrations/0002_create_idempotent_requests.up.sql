CREATE TABLE idempotent_requests (
    idempotency_key VARCHAR(255) NOT NULL,
    consumer_id     VARCHAR(100) NOT NULL,
    request_hash    VARCHAR(64)  NOT NULL,
    response_body   BYTEA        NOT NULL,
    status_code     INTEGER      NOT NULL,
    expires_at      TIMESTAMPTZ  NOT NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    PRIMARY KEY (idempotency_key, consumer_id)
);
CREATE INDEX idx_idempotent_expires ON idempotent_requests(expires_at);
