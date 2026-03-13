-- +goose Up
CREATE TABLE users (
    id          TEXT PRIMARY KEY,
    username    TEXT UNIQUE NOT NULL,
    display_name TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    role        TEXT NOT NULL DEFAULT 'user',
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE submissions (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id),
    problem_id  TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending',
    language    TEXT NOT NULL,
    submitted_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP,
    duration_ms  INTEGER,
    total_tests  INTEGER,
    passed_tests INTEGER,
    result_json  TEXT
);

CREATE INDEX idx_submissions_user ON submissions(user_id, submitted_at DESC);
CREATE INDEX idx_submissions_problem ON submissions(problem_id, user_id);

CREATE TABLE submission_files (
    id            TEXT PRIMARY KEY,
    submission_id TEXT NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
    file_path     TEXT NOT NULL,
    content       TEXT NOT NULL
);

CREATE INDEX idx_submission_files ON submission_files(submission_id);

CREATE TABLE user_problem_state (
    user_id      TEXT NOT NULL REFERENCES users(id),
    problem_id   TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'unseen',
    best_submission_id TEXT REFERENCES submissions(id),
    hints_used   INTEGER NOT NULL DEFAULT 0,
    bookmarked   INTEGER NOT NULL DEFAULT 0,
    notes        TEXT,
    first_seen_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    solved_at     TIMESTAMP,
    PRIMARY KEY (user_id, problem_id)
);

CREATE TABLE generation_jobs (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id),
    prompt      TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending',
    problem_id  TEXT,
    error       TEXT,
    attempts    INTEGER NOT NULL DEFAULT 0,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP
);

CREATE TABLE daily_activity (
    user_id     TEXT NOT NULL REFERENCES users(id),
    date        DATE NOT NULL,
    problems_attempted INTEGER NOT NULL DEFAULT 0,
    problems_solved    INTEGER NOT NULL DEFAULT 0,
    submissions_count  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, date)
);

-- +goose Down
DROP TABLE daily_activity;
DROP TABLE generation_jobs;
DROP TABLE user_problem_state;
DROP TABLE submission_files;
DROP TABLE submissions;
DROP TABLE users;
