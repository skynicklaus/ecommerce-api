-- name: CountPlatformAdmins :one
SELECT
    COUNT(*)
FROM
    members m
    JOIN organizations o ON o.id = m.organization_id
WHERE
    o.type = 'platform';

-- name: CreateMember :one
INSERT INTO
    members (identity_id, organization_id)
VALUES
    ($1, $2)
RETURNING
    *;

-- name: GetMemberByIdentityID :one
SELECT
    id,
    identity_id,
    organization_id,
    created_at
FROM
    members
WHERE
    identity_id = $1
ORDER BY
    created_at ASC
LIMIT
    1;
