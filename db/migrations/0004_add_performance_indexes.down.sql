-- Remove performance indexes

DROP INDEX IF EXISTS idx_replies_inreplyto;

DROP INDEX IF EXISTS idx_activitystream_id_type;
DROP INDEX IF EXISTS idx_activitystream_attachment;
DROP INDEX IF EXISTS idx_activitystream_preview;

DROP INDEX IF EXISTS idx_cacheactivitystream_id_type;
DROP INDEX IF EXISTS idx_cacheactivitystream_attachment;
DROP INDEX IF EXISTS idx_cacheactivitystream_preview;