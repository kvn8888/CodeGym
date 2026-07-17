-- Run only after deploying the LLM-only fallback behavior from issue #109.
-- This removes legacy derived profiles that have no successful model provenance.
-- Raw memory_events are intentionally preserved so the next successful synthesis
-- can rebuild a curated profile from the original evidence.

BEGIN;

SELECT workspace_id, user_id, updated_at
FROM user_memory_profiles
WHERE NULLIF(profile->'provenance'->>'provider', '') IS NULL
ORDER BY updated_at DESC;

DELETE FROM user_memory_profiles
WHERE NULLIF(profile->'provenance'->>'provider', '') IS NULL;

COMMIT;
