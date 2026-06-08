-- name: AddConversationMember :one
INSERT INTO chat_schema.conversation_members (conversation_id, user_id, role)
VALUES ($1, $2, $3)
RETURNING id, conversation_id, user_id, role, joined_at;

-- name: GetConversationMembers :many
SELECT id, conversation_id, user_id, role, joined_at
FROM chat_schema.conversation_members
WHERE conversation_id = $1
ORDER BY joined_at ASC;

-- name: IsConversationMember :one
SELECT EXISTS (
    SELECT 1 FROM chat_schema.conversation_members
    WHERE conversation_id = $1 AND user_id = $2
) AS is_member;

-- name: GetMemberRole :one
SELECT role FROM chat_schema.conversation_members
WHERE conversation_id = $1 AND user_id = $2;

-- name: CountConversationMembers :one
SELECT COUNT(*) FROM chat_schema.conversation_members
WHERE conversation_id = $1;