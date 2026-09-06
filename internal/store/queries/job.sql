-- name: EnqueueJob :exec
INSERT INTO jobs (id, owner_id, type, status, payload, created_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: UpdateJobStatus :exec
UPDATE jobs SET status = ? WHERE id = ? AND owner_id = ?;

-- name: UpdateJobProgress :exec
-- Updates status and payload together so a long-running job (e.g. a migration
-- import) can persist progress counts in the payload JSON without a schema
-- change. See internal/importers batch progress.
UPDATE jobs SET status = ?, payload = ? WHERE id = ? AND owner_id = ?;

-- name: ListJobs :many
SELECT id, owner_id, type, status, payload, created_at
FROM jobs
WHERE owner_id = ?
ORDER BY created_at DESC
LIMIT ? OFFSET ?;
