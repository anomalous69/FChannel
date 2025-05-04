-- Add performance indexes for replies and activity streams

-- Replies index
CREATE INDEX IF NOT EXISTS idx_replies_inreplyto ON replies(inreplyto, id);

-- ActivityStream indexes
CREATE INDEX IF NOT EXISTS idx_activitystream_id_type ON activitystream(id, type);
CREATE INDEX IF NOT EXISTS idx_activitystream_attachment ON activitystream(attachment) 
    WHERE attachment IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_activitystream_preview ON activitystream(preview) 
    WHERE preview IS NOT NULL;

-- CacheActivityStream indexes
CREATE INDEX IF NOT EXISTS idx_cacheactivitystream_id_type ON cacheactivitystream(id, type);
CREATE INDEX IF NOT EXISTS idx_cacheactivitystream_attachment ON cacheactivitystream(attachment) 
    WHERE attachment IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_cacheactivitystream_preview ON cacheactivitystream(preview) 
    WHERE preview IS NOT NULL;