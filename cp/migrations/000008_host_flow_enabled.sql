-- +goose Up
ALTER TABLE hosts ADD COLUMN flow_events_enabled BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE hosts DROP COLUMN flow_events_enabled;
