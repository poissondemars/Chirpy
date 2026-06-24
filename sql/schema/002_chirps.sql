-- +goose Up
CREATE TABLE chirps (
    id UUID PRIMARY KEY,
    created_at TIMESTAMP,
    updated_at TIMESTAMP,
    body TEXT,
    user_id UUID,
    CONSTRAINT fk_chirps_users
    FOREIGN KEY (user_id) REFERENCES users(id)
);

-- +goose Down
ALTER TABLE chirps
DROP CONSTRAINT fk_chirps_users;

DROP TABLE chirps;