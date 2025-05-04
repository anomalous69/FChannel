-- Rollback for 0002_actor_column_rename.up.sql
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='actor' AND column_name='name') THEN
        ALTER TABLE actor RENAME COLUMN name TO preferedusername;
        ALTER TABLE actor ALTER COLUMN preferedusername TYPE varchar(100);
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='actor' AND column_name='preferredusername') THEN
        ALTER TABLE actor RENAME COLUMN preferredusername TO name;
        ALTER TABLE actor ALTER COLUMN name TYPE varchar(50);
    END IF;
END$$;
