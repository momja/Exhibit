-- +goose Up
-- Older experimental native recipes used the same origin-scoped HTTP Basic
-- shape with a private endpoint in their encrypted credential. Convert by
-- shape rather than keeping a provider-specific identifier in the product.
-- The generic engine recovers the private base from that encrypted endpoint
-- and its existing public alias. IDs, grants, retained data custody, encrypted
-- secrets and request budgets remain intact.
UPDATE connections
SET provider='basic', recipe_json=json_set(recipe_json,'$.sensitivity','personal')
WHERE kind='native' AND provider NOT IN ('api-key','bearer','basic')
  AND json_extract(recipe_json,'$.header')='Authorization'
  AND json_extract(recipe_json,'$.scheme')='Basic ';

-- Earlier cache entries contained the first viewer's scrubbed response. Inert
-- handles are viewer-local and expire, so those entries cannot be reused by a
-- connection-wide cache. New entries seal original bytes and a minimal secret
-- snapshot and are scrubbed for the current viewer on every delivery.
DELETE FROM connection_cache;

-- +goose Down
-- Provider identity cannot be reconstructed safely; generic Basic is retained.
SELECT 1;
