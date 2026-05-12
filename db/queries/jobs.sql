-- name: EnqueueJob :one
INSERT INTO jobs (type, payload, available_at)
VALUES ($1, $2, $3)
RETURNING id, type, payload, status, attempts, max_attempts, available_at, locked_at, locked_by, last_error, created_at, updated_at;

-- name: ClaimJob :one
UPDATE jobs
SET status = 'running',
    attempts = attempts + 1,
    locked_at = now(),
    locked_by = $1,
    updated_at = now()
WHERE id = (
    SELECT id
    FROM jobs
    WHERE status = 'queued'
        AND available_at <= now()
    ORDER BY available_at, created_at
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
RETURNING id, type, payload, status, attempts, max_attempts, available_at, locked_at, locked_by, last_error, created_at, updated_at;

-- name: CompleteJob :exec
UPDATE jobs
SET status = 'succeeded',
    updated_at = now()
WHERE id = $1;

-- name: FailJob :exec
UPDATE jobs
SET status = CASE WHEN attempts >= max_attempts THEN 'failed' ELSE 'queued' END,
    last_error = $2,
    available_at = $3,
    locked_at = NULL,
    locked_by = NULL,
    updated_at = now()
WHERE id = $1;
