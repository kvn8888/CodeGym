-- name: UpsertDailyActivity :exec
INSERT INTO daily_activity (user_id, date, problems_attempted, problems_solved, submissions_count)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (user_id, date) DO UPDATE SET
    problems_attempted = daily_activity.problems_attempted + excluded.problems_attempted,
    problems_solved = daily_activity.problems_solved + excluded.problems_solved,
    submissions_count = daily_activity.submissions_count + excluded.submissions_count;

-- name: GetUserActivity :many
SELECT * FROM daily_activity WHERE user_id = ? ORDER BY date DESC LIMIT ?;

-- name: GetUserProblemState :one
SELECT * FROM user_problem_state WHERE user_id = ? AND problem_id = ?;

-- name: UpsertUserProblemState :exec
INSERT INTO user_problem_state (user_id, problem_id, status)
VALUES (?, ?, ?)
ON CONFLICT (user_id, problem_id) DO UPDATE SET status = excluded.status;

-- name: UpdateUserProblemSolved :exec
UPDATE user_problem_state
SET status = 'solved', best_submission_id = ?, solved_at = CURRENT_TIMESTAMP
WHERE user_id = ? AND problem_id = ?;

-- name: CountUserProgress :one
SELECT
    SUM(CASE WHEN status IN ('attempted', 'solved') THEN 1 ELSE 0 END) as attempted,
    SUM(CASE WHEN status = 'solved' THEN 1 ELSE 0 END) as solved
FROM user_problem_state WHERE user_id = ?;
