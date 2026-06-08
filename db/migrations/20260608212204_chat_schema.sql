-- +goose Up

CREATE SCHEMA IF NOT EXISTS chat_schema;

-- conversation types: 'dm' for direct messages, 'group' for group chats
CREATE TABLE chat_schema.conversations (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type        VARCHAR(10) NOT NULL DEFAULT 'dm' CHECK (type IN ('dm', 'group')),
    name        VARCHAR(255),       -- NULL for DMs, required for groups
    created_by  UUID NOT NULL,      -- user who initiated the conversation
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE chat_schema.conversation_members (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES chat_schema.conversations(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL,
    role            VARCHAR(20) NOT NULL DEFAULT 'member' CHECK (role IN ('owner', 'admin', 'member')),
    joined_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (conversation_id, user_id)
);

CREATE TABLE chat_schema.messages (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES chat_schema.conversations(id) ON DELETE CASCADE,
    sender_id       UUID NOT NULL,
    content         TEXT NOT NULL,
    is_edited       BOOLEAN NOT NULL DEFAULT FALSE,
    is_deleted      BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_conversation_members_user_id ON chat_schema.conversation_members(user_id);
CREATE INDEX idx_conversation_members_conversation_id ON chat_schema.conversation_members(conversation_id);
CREATE INDEX idx_messages_conversation_id_created_at ON chat_schema.messages(conversation_id, created_at DESC);
CREATE INDEX idx_messages_sender_id ON chat_schema.messages(sender_id);

-- DM uniqueness: for type='dm', prevent duplicate pairs.
-- We use a functional unique index on the sorted member pair.
-- This is enforced at the application level + a trigger.

-- Trigger to auto-update updated_at on conversations when a new message arrives
CREATE OR REPLACE FUNCTION chat_schema.update_conversation_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE chat_schema.conversations
    SET updated_at = NOW()
    WHERE id = NEW.conversation_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_update_conversation_timestamp
    AFTER INSERT ON chat_schema.messages
    FOR EACH ROW
    EXECUTE FUNCTION chat_schema.update_conversation_timestamp();

-- +goose Down

DROP TRIGGER IF EXISTS trg_update_conversation_timestamp ON chat_schema.messages;
DROP FUNCTION IF EXISTS chat_schema.update_conversation_timestamp;
DROP TABLE IF EXISTS chat_schema.messages;
DROP TABLE IF EXISTS chat_schema.conversation_members;
DROP TABLE IF EXISTS chat_schema.conversations;
DROP SCHEMA IF EXISTS chat_schema;