-- +goose Up
ALTER TABLE hosts
    ADD COLUMN desired_generation INT NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE hosts
    DROP COLUMN desired_generation;
