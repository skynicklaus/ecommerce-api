-- name: CreateOrderStatusHistory :one
INSERT INTO
    order_status_histories (
        order_id,
        from_status,
        to_status,
        note,
        actor_type,
        changed_by_member_id,
        metadata
    )
VALUES
    (
        sqlc.arg('order_id')::UUID,
        sqlc.narg('from_status')::TEXT,
        sqlc.arg('to_status')::TEXT,
        sqlc.narg('note')::TEXT,
        sqlc.arg('actor_type')::TEXT,
        sqlc.narg('changed_by_member_id')::UUID,
        sqlc.narg('metadata')::JSONB
    )
RETURNING
    *;

-- name: ListOrderStatusHistory :many
SELECT
    *
FROM
    order_status_histories
WHERE
    order_id = sqlc.arg('order_id')::UUID
ORDER BY
    created_at DESC,
    id DESC;
