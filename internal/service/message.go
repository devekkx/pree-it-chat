package service

import (
	"context"
	"fmt"
	"time"

	"github.com/devekkx/pree-it-chat/db/sqlc"
	"github.com/devekkx/pree-it-chat/internal/event"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type MessageService struct {
	queries   *sqlc.Queries
	publisher *event.Publisher
	logger    *zap.Logger
}

func NewMessageService(queries *sqlc.Queries, publisher *event.Publisher, logger *zap.Logger) *MessageService {
	return &MessageService{
		queries:   queries,
		publisher: publisher,
		logger:    logger,
	}
}

type MessageResponse struct {
	ID             uuid.UUID `json:"id"`
	ConversationID uuid.UUID `json:"conversation_id"`
	SenderID       uuid.UUID `json:"sender_id"`
	Content        string    `json:"content"`
	IsEdited       bool      `json:"is_edited"`
	CreatedAt      string    `json:"created_at"`
	UpdatedAt      string    `json:"updated_at"`
}

type ListMessagesParams struct {
	ConversationID uuid.UUID
	Cursor         *time.Time // nil = first page
	Limit          int32
}

func (s *MessageService) SendMessage(ctx context.Context, conversationID, senderID uuid.UUID, content string) (*MessageResponse, error) {
	ctx, span := tracer.Start(ctx, "MessageService.SendMessage")
	defer span.End()

	if content == "" {
		return nil, fmt.Errorf("message content cannot be empty")
	}

	if len(content) > 4000 {
		return nil, fmt.Errorf("message content exceeds 4000 character limit")
	}

	// Verify sender is a member
	isMember, err := s.queries.IsConversationMember(ctx, sqlc.IsConversationMemberParams{
		ConversationID: conversationID,
		UserID:         senderID,
	})
	if err != nil {
		return nil, fmt.Errorf("check membership: %w", err)
	}
	if !isMember {
		return nil, fmt.Errorf("not a member of this conversation")
	}

	msg, err := s.queries.CreateMessage(ctx, sqlc.CreateMessageParams{
		ConversationID: conversationID,
		SenderID:       senderID,
		Content:        content,
	})
	if err != nil {
		return nil, fmt.Errorf("create message: %w", err)
	}

	// Publish event for realtime fanout
	s.publisher.Publish(ctx, event.SubjectMessageCreated, event.MessageEvent{
		MessageID:      msg.ID,
		ConversationID: msg.ConversationID,
		SenderID:       msg.SenderID,
		Content:        msg.Content,
		IsEdited:       msg.IsEdited,
		CreatedAt:      msg.CreatedAt,
		UpdatedAt:      msg.UpdatedAt,
	})

	return toMessageResponse(msg), nil
}

func (s *MessageService) ListMessages(ctx context.Context, userID uuid.UUID, params ListMessagesParams) ([]MessageResponse, error) {
	ctx, span := tracer.Start(ctx, "MessageService.ListMessages")
	defer span.End()

	// Verify membership
	isMember, err := s.queries.IsConversationMember(ctx, sqlc.IsConversationMemberParams{
		ConversationID: params.ConversationID,
		UserID:         userID,
	})
	if err != nil {
		return nil, fmt.Errorf("check membership: %w", err)
	}
	if !isMember {
		return nil, fmt.Errorf("not a member of this conversation")
	}

	var messages []sqlc.ChatSchemaMessage

	if params.Cursor != nil {
		messages, err = s.queries.ListMessages(ctx, sqlc.ListMessagesParams{
			ConversationID: params.ConversationID,
			CreatedAt:      *params.Cursor,
			Limit:          params.Limit,
		})
	} else {
		messages, err = s.queries.ListMessagesInitial(ctx, sqlc.ListMessagesInitialParams{
			ConversationID: params.ConversationID,
			Limit:          params.Limit,
		})
	}
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}

	results := make([]MessageResponse, len(messages))
	for i, m := range messages {
		results[i] = *toMessageResponse(m)
	}

	return results, nil
}

func (s *MessageService) EditMessage(ctx context.Context, messageID, userID uuid.UUID, content string) (*MessageResponse, error) {
	ctx, span := tracer.Start(ctx, "MessageService.EditMessage")
	defer span.End()

	if content == "" {
		return nil, fmt.Errorf("message content cannot be empty")
	}

	if len(content) > 4000 {
		return nil, fmt.Errorf("message content exceeds 4000 character limit")
	}

	// Verify ownership
	existing, err := s.queries.GetMessageByID(ctx, messageID)
	if err != nil {
		return nil, fmt.Errorf("get message: %w", err)
	}

	if existing.SenderID != userID {
		return nil, fmt.Errorf("only the sender can edit this message")
	}

	if existing.IsDeleted {
		return nil, fmt.Errorf("cannot edit a deleted message")
	}

	msg, err := s.queries.UpdateMessageContent(ctx, sqlc.UpdateMessageContentParams{
		ID:      messageID,
		Content: content,
	})
	if err != nil {
		return nil, fmt.Errorf("update message: %w", err)
	}

	s.publisher.Publish(ctx, event.SubjectMessageUpdated, event.MessageEvent{
		MessageID:      msg.ID,
		ConversationID: msg.ConversationID,
		SenderID:       msg.SenderID,
		Content:        msg.Content,
		IsEdited:       msg.IsEdited,
		CreatedAt:      msg.CreatedAt,
		UpdatedAt:      msg.UpdatedAt,
	})

	return toMessageResponse(msg), nil
}

func (s *MessageService) DeleteMessage(ctx context.Context, messageID, userID uuid.UUID) error {
	ctx, span := tracer.Start(ctx, "MessageService.DeleteMessage")
	defer span.End()

	existing, err := s.queries.GetMessageByID(ctx, messageID)
	if err != nil {
		return fmt.Errorf("get message: %w", err)
	}

	if existing.SenderID != userID {
		return fmt.Errorf("only the sender can delete this message")
	}

	if existing.IsDeleted {
		return fmt.Errorf("message already deleted")
	}

	if err := s.queries.SoftDeleteMessage(ctx, messageID); err != nil {
		return fmt.Errorf("soft delete message: %w", err)
	}

	s.publisher.Publish(ctx, event.SubjectMessageDeleted, event.MessageEvent{
		MessageID:      existing.ID,
		ConversationID: existing.ConversationID,
		SenderID:       existing.SenderID,
		CreatedAt:      existing.CreatedAt,
		UpdatedAt:      time.Now(),
	})

	return nil
}

func toMessageResponse(m sqlc.ChatSchemaMessage) *MessageResponse {
	return &MessageResponse{
		ID:             m.ID,
		ConversationID: m.ConversationID,
		SenderID:       m.SenderID,
		Content:        m.Content,
		IsEdited:       m.IsEdited,
		CreatedAt:      m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:      m.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
