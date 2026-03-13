-- name: CreateSubmission :one
INSERT INTO submissions (id, user_id, problem_id, status, language)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetSubmission :one
SELECT * FROM submissions WHERE id = ?;

-- name: UpdateSubmissionResult :exec
UPDATE submissions
SET status = ?, completed_at = CURRENT_TIMESTAMP, duration_ms = ?, total_tests = ?, passed_tests = ?, result_json = ?
WHERE id = ?;

-- name: ListSubmissionsByUser :many
SELECT * FROM submissions WHERE user_id = ? ORDER BY submitted_at DESC LIMIT ? OFFSET ?;

-- name: ListSubmissionsByProblem :many
SELECT * FROM submissions WHERE problem_id = ? AND user_id = ? ORDER BY submitted_at DESC;

-- name: CreateSubmissionFile :exec
INSERT INTO submission_files (id, submission_id, file_path, content)
VALUES (?, ?, ?, ?);

-- name: GetSubmissionFiles :many
SELECT * FROM submission_files WHERE submission_id = ?;
