-- name: AssignRoleToMember :exec
INSERT INTO member_roles (
    member_id
    , role_id
    , assigned_by
) VALUES ($1, $2, $3);
