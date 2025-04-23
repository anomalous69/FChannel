-- Migration for legacy servers: rename columns in actor table
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='actor' AND column_name='preferedusername') THEN
        -- Rename preferedusername to name (if name exists, rename it first)
        IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='actor' AND column_name='name') THEN
            ALTER TABLE actor RENAME COLUMN name TO preferredusername;
        END IF;
        ALTER TABLE actor RENAME COLUMN preferedusername TO name;
    END IF;
    -- Set correct types for legacy columns
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='actor' AND column_name='preferedusername') THEN
        ALTER TABLE actor ALTER COLUMN preferedusername TYPE varchar(50);
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='actor' AND column_name='name') THEN
        ALTER TABLE actor ALTER COLUMN name TYPE varchar(100);
    END IF;
END$$;
