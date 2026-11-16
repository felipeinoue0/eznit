-- Before #358 the upload and replace paths dropped the extension with
-- strings.ReplaceAll, which removes every occurrence: "notes.txt.txt" was
-- stored as name "notes". The original name survives as the last segment of
-- the stored path, <root>/<user-id>/<file-id>/<version>_<original>, and uploads
-- refuse "/" and "\" in names, so it can be read back exactly.
--
-- Only rows whose path has that layout under the row's own id are touched.
-- The earliest layout, <root>/<unix-time>_<already-cut name>, has no file-id
-- folder and nothing to recover; it is left alone. Both separators are
-- accepted, for a server that ran on Windows.
--
-- The split mirrors splitName in internal/domain/files/service.go: the last
-- extension only, without its dot.
WITH recovered AS (
    SELECT id,
           regexp_replace(regexp_replace(path, '^.*[/\\]', ''), '^[0-9]+_', '') AS original
    FROM files
    WHERE path ~ ('[/\\]' || id::text || '[/\\][0-9]+_[^/\\]+$')
),
split AS (
    SELECT id,
           CASE WHEN position('.' IN original) > 0
                THEN regexp_replace(original, '\.[^.]*$', '')
                ELSE original END AS name,
           CASE WHEN position('.' IN original) > 0
                THEN substring(original FROM '\.([^.]*)$')
                ELSE '' END AS ext
    FROM recovered
)
UPDATE files AS f
SET name = s.name, ext = s.ext
FROM split AS s
WHERE f.id = s.id
  AND (f.name IS DISTINCT FROM s.name OR f.ext IS DISTINCT FROM s.ext);
