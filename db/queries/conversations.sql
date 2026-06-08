-- name: CreateConversation :one
INSERT INTO chat_schema.conversations (type, name, created_by)
VALUES ($1, $2, $3)
RETURNING id, type, name, created_by, created_at, updated_at;

-- name: GetConversationByID :one
SELECT id, type, name, created_by, created_at, updated_at
FROM chat_schema.conversations
WHERE id = $1;

-- name: ListConversationsByUserID :many
SELECT c.id, c.type, c.name, c.created_by, c.created_at, c.updated_at
FROM chat_schema.conversations c
INNER JOIN chat_schema.conversation_members cm ON cm.conversation_id = c.id
WHERE cm.user_id = $1
ORDER BY c.updated_at DESC
LIMIT $2 OFFSET $3;

-- name: CountConversationsByUserID :one
SELECT COUNT(*)
FROM chat_schema.conversations c
INNER JOIN chat_schema.conversation_members cm ON cm.conversation_id = c.id
WHERE cm.user_id = $1;

-- name: FindDMConversation :one
-- Find an existing DM between exactly two users
SELECT c.id, c.type, c.name, c.created_by, c.created_at, c.updated_at
FROM chat_schema.conversations c
WHERE c.type = 'dm'
  AND c.id IN (
      SELECT conversation_id FROM chat_schema.conversation_members WHERE user_id = $1
  )
  AND c.id IN (
      SELECT conversation_id FROM chat_schema.conversation_members WHERE user_id = $2
  );