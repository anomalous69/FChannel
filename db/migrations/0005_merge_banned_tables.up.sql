-- Add new columns to bannedmedia
ALTER TABLE bannedmedia ADD COLUMN IF NOT EXISTS phash NUMERIC UNIQUE;
ALTER TABLE bannedmedia ADD COLUMN IF NOT EXISTS created_at TIMESTAMP DEFAULT timezone('utc', now()) NOT NULL;

-- Add unique constraint (removed IF NOT EXISTS)
ALTER TABLE bannedmedia ADD CONSTRAINT bannedmedia_hash_unique UNIQUE (hash);

-- Copy data without type casting to preserve values and only drop table if copy succeeds
DO $$
DECLARE
    copied_rows integer;
    table_exists boolean;
BEGIN
    SELECT EXISTS (
        SELECT FROM information_schema.tables 
        WHERE table_schema = 'public' 
        AND table_name = 'bannedimages'
    ) INTO table_exists;

    IF table_exists THEN
        WITH inserted AS (
            INSERT INTO bannedmedia (phash, created_at)
            SELECT phash, timezone('utc', now())
            FROM bannedimages
            ON CONFLICT DO NOTHING
            RETURNING 1
        )
        SELECT COUNT(*) INTO copied_rows FROM inserted;
        
        IF copied_rows > 0 THEN
            DROP TABLE bannedimages;
        END IF;
    END IF;
END $$;