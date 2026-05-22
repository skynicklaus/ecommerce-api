-- name: CreateSession :one
INSERT INTO sessions (
    identity_id,
    token,
    service,
    expires_at,
    ip_address,
    user_agent
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING *;

-- name: GetSessionWithIdentity :one
SELECT 
    s.id AS session_id,
    s.identity_id,
    s.token,
    s.service,
    s.expires_at,
    s.ip_address,
    s.user_agent,
    s.created_at,
    s.updated_at,
    i.type AS identity_type
FROM sessions s
JOIN identities i ON s.identity_id = i.id
WHERE s.token = $1 AND s.expires_at > NOW() LIMIT 1;

-- name: RenewSession :exec
UPDATE sessions
SET
    expires_at = sqlc.arg('expires_at')::TIMESTAMPTZ,
    updated_at = NOW()
WHERE
    token = sqlc.arg('token');

-- name: DeleteSessionByToken :exec
DELETE FROM sessions
WHERE token = $1;

-- name: DeleteAllSessionsByIdentity :exec
DELETE FROM sessions
WHERE identity_id = $1;

-- name: ListSessionsByIdentity :many
SELECT id, service, ip_address, user_agent, created_at, updated_at, expires_at
FROM sessions
WHERE identity_id = $1 AND expires_at > NOW()
ORDER BY updated_at DESC;

-- name: DeleteSessionByIDAndIdentity :exec
DELETE FROM sessions
WHERE id = $1 AND identity_id = $2;

-- name: DeleteAllOtherSessionsByIdentity :exec
DELETE FROM sessions
WHERE identity_id = $1 AND token <> $2;
