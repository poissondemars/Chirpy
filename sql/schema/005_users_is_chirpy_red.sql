-- +goose Up
ALTER TABLE users 
ADD is_chirpy_red BOOL DEFAULT false NOT NULL ;

-- +goose Down
ALTER TABLE users
DROP COLUMN is_chirpy_red;