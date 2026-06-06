-- +goose Up

ALTER TABLE certificates
    DROP CONSTRAINT certificates_host_id_fkey,
    ADD  CONSTRAINT certificates_host_id_fkey
         FOREIGN KEY (host_id) REFERENCES hosts(id) ON DELETE CASCADE;

ALTER TABLE bootstrap_tokens
    DROP CONSTRAINT bootstrap_tokens_target_host_id_fkey,
    ADD  CONSTRAINT bootstrap_tokens_target_host_id_fkey
         FOREIGN KEY (target_host_id) REFERENCES hosts(id) ON DELETE SET NULL;

-- +goose Down

ALTER TABLE certificates
    DROP CONSTRAINT certificates_host_id_fkey,
    ADD  CONSTRAINT certificates_host_id_fkey
         FOREIGN KEY (host_id) REFERENCES hosts(id);

ALTER TABLE bootstrap_tokens
    DROP CONSTRAINT bootstrap_tokens_target_host_id_fkey,
    ADD  CONSTRAINT bootstrap_tokens_target_host_id_fkey
         FOREIGN KEY (target_host_id) REFERENCES hosts(id);
