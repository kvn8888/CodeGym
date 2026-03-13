-- name: CreateGenerationJob :one
INSERT INTO generation_jobs (id, user_id, prompt, status)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: GetGenerationJob :one
SELECT * FROM generation_jobs WHERE id = ?;

-- name: UpdateGenerationJobStatus :exec
UPDATE generation_jobs
SET status = ?, problem_id = ?, error = ?, attempts = ?, completed_at = CURRENT_TIMESTAMP
WHERE id = ?;
