DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM refresh_tokens) THEN
        RAISE EXCEPTION 'cannot roll back migration 018: % refresh_tokens rows exist (including revoked). Delete all refresh_tokens rows manually before rolling back.',
            (SELECT COUNT(*) FROM refresh_tokens);
    END IF;
END$$;

DROP TABLE IF EXISTS refresh_tokens;
