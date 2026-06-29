-- +goose Up
CREATE TABLE refresh_tokens (
    token TEXT PRIMARY KEY,
    created_at TIMESTAMP,
    updated_at TIMESTAMP,
    user_id UUID,
    expires_at TIMESTAMP,
    revoked_at TIMESTAMP,
    CONSTRAINT fk_refresh_tokens_users
    FOREIGN KEY (user_id) REFERENCES users(id)
);

-- +goose Down
DROP TABLE refresh_tokens;