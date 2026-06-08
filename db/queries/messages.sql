-- name: CreateMessage :one
INSERT INTO chat_schema.messages (conversation_id, sender_id, content)
VALUES ($1, $2, $3)
RETURNING id, conversation_id, sender_id, content, is_edited, is_deleted, created_at, updated_at;

-- name: GetMessageByID :one
SELECT id, conversation_id, sender_id, content, is_edited, is_deleted, created_at, updated_at
FROM chat_schema.messages
WHERE id = $1;

-- name: ListMessages :many
-- Cursor-based pagination: fetch messages older than the cursor timestamp
SELECT id, conversation_id, sender_id, content, is_edited, is_deleted, created_at, updated_at
FROM chat_schema.messages
WHERE conversation_id = $1
  AND is_deleted = FALSE
  AND created_at < $2
ORDER BY created_at DESC
LIMIT $3;

-- name: ListMessagesInitial :many
-- First page: no cursor, just latest N messages
SELECT id, conversation_id, sender_id, content, is_edited, is_deleted, created_at, updated_at
FROM chat_schema.messages
WHERE conversation_id = $1
  AND is_deleted = FALSE
ORDER BY created_at DESC
LIMIT $2;

-- name: UpdateMessageContent :one
UPDATE chat_schema.messages
SET content = $2, is_edited = TRUE, updated_at = NOW()
WHERE id = $1 AND is_deleted = FALSE
RETURNING id, conversation_id, sender_id, content, is_edited, is_deleted, created_at, updated_at;

-- name: SoftDeleteMessage :exec
UPDATE chat_schema.messages
SET is_deleted = TRUE, content = '', updated_at = NOW()
WHERE id = $1;

-- name: GetLastMessage :one
SELECT id, conversation_id, sender_id, content, is_edited, is_deleted, created_at, updated_at
FROM chat_schema.messages
WHERE conversation_id = $1
  AND is_deleted = FALSE
ORDER BY created_at DESC
LIMIT 1;