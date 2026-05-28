-- Precondition: zero users rows. There is no legitimate path that creates a
-- row before 017 applies (Register/Login routes are unregistered in PR 1;
-- PR 2 has not yet introduced the Firebase upsert). Any row here is an
-- operational anomaly; auto-linking by email is prohibited.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM users) THEN
        RAISE EXCEPTION 'migration 017: users table must be empty before Firebase rollout (found % rows). Inspect and remove pre-existing rows manually rather than auto-linking.',
            (SELECT COUNT(*) FROM users);
    END IF;
END$$;

ALTER TABLE users ALTER COLUMN password_hash DROP NOT NULL;
ALTER TABLE users ALTER COLUMN name          DROP NOT NULL;

ALTER TABLE users ADD COLUMN firebase_uid  TEXT NOT NULL UNIQUE;
ALTER TABLE users ADD COLUMN display_name  TEXT;
ALTER TABLE users ADD COLUMN photo_url     TEXT;
ALTER TABLE users ADD COLUMN last_login_at TIMESTAMPTZ;   -- nullable, no default
