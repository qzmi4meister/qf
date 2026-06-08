-- +goose Up
ALTER TABLE hosts ADD CONSTRAINT hosts_tenant_hostname_unique UNIQUE (tenant_id, hostname);

-- +goose Down
ALTER TABLE hosts DROP CONSTRAINT hosts_tenant_hostname_unique;
