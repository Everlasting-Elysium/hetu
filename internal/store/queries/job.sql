-- name: EnqueueJob :exec
INSERT INTO jobs (id, owner_id, type, status, payload, created_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: UpdateJobStatus :exec
UPDATE jobs SET status = ? WHERE id = ? AND owner_id = ?;

-- name: ListJobs :many
SELECT id, owner_id, type, status, payload, created_at
FROM jobs
WHERE owner_id = ?
ORDER BY created_at DESC
LIMIT ? OFFSET ?;
