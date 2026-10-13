CREATE TABLE IF NOT EXISTS projects (
id BIGSERIAL PRIMARY KEY, title TEXT NOT NULL, description TEXT NOT NULL,
url TEXT NOT NULL DEFAULT '', tags TEXT[] NOT NULL DEFAULT '{}', created_at TIMESTAMPTZ NOT NULL DEFAULT now());
-- Existing projects share order 0, preserving their previous newest-first order.
ALTER TABLE projects ADD COLUMN IF NOT EXISTS sort_order INTEGER NOT NULL DEFAULT 0 CHECK (sort_order >= 0);
CREATE TABLE IF NOT EXISTS contact_messages (
id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, email TEXT NOT NULL, message TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now());
-- Historical messages are not mailed; new submissions explicitly opt into the queue.
ALTER TABLE contact_messages ADD COLUMN IF NOT EXISTS email_pending BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE contact_messages ADD COLUMN IF NOT EXISTS email_attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE contact_messages ADD COLUMN IF NOT EXISTS email_next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE contact_messages ADD COLUMN IF NOT EXISTS email_sent_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS contact_email_queue ON contact_messages(email_next_attempt_at,id) WHERE email_pending;
CREATE TABLE IF NOT EXISTS sessions (token_hash TEXT PRIMARY KEY, expires_at TIMESTAMPTZ NOT NULL);
