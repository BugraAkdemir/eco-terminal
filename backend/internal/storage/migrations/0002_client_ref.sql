-- Idempotent observation creates (offline sync): a device-generated client_ref is unique per user.
ALTER TABLE observations ADD COLUMN client_ref TEXT;
CREATE UNIQUE INDEX idx_obs_client_ref ON observations(user_id, client_ref) WHERE client_ref IS NOT NULL;
