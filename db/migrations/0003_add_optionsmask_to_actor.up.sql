-- Add OptionsMask column to actor table with OptionTripcode enabled by default (value 4)
ALTER TABLE actor ADD COLUMN IF NOT EXISTS optionsmask INTEGER NOT NULL DEFAULT 4;
UPDATE actor SET optionsmask = 0 WHERE preferredusername = 'main';