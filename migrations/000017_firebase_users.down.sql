DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM users WHERE firebase_uid IS NOT NULL) THEN
        RAISE EXCEPTION 'cannot roll back migration 017: % rows have firebase_uid set. Delete Firebase-linked users manually before rolling back.',
            (SELECT COUNT(*) FROM users WHERE firebase_uid IS NOT NULL);
    END IF;
END$$;

ALTER TABLE users DROP COLUMN IF EXISTS last_login_at;
ALTER TABLE users DROP COLUMN IF EXISTS photo_url;
ALTER TABLE users DROP COLUMN IF EXISTS display_name;
ALTER TABLE users DROP COLUMN IF EXISTS firebase_uid;

ALTER TABLE users ALTER COLUMN name          SET NOT NULL;
ALTER TABLE users ALTER COLUMN password_hash SET NOT NULL;
