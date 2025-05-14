-- Recreate bannedimages table
CREATE TABLE IF NOT EXISTS bannedimages(
    phash numeric UNIQUE NOT NULL
);

-- Copy data from bannedmedia to bannedimages
INSERT INTO bannedimages (phash)
SELECT phash
FROM bannedmedia 
WHERE phash IS NOT NULL
ON CONFLICT DO NOTHING;

-- Remove constraints and columns from bannedmedia
ALTER TABLE bannedmedia DROP CONSTRAINT IF EXISTS bannedmedia_hash_unique;
ALTER TABLE bannedmedia DROP COLUMN IF EXISTS phash;
ALTER TABLE bannedmedia DROP COLUMN IF EXISTS created_at;
