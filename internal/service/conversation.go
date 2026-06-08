package service

import (
	"context"
	"fmt"

	"github.com/devekkx/pree-it-chat/db/sqlc"
	"github.com/devekkx/pree-it-chat/internal/userclient"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
)

var tracer = otel.Tracer("chat-service")

type ConversationService struct {
	pool       *pgxpool.Pool
	queries    *sqlc.Queries
	userClient *userclient.Client
	logger     *zap.Logger
}

func NewConversationService(pool *pgxpool.Pool, queries *sqlc.Queries, userClient *userclient.Client, logger *zap.Logger) *ConversationService {
	return &ConversationService{
		pool:       pool,
		queries:    queries,
		userClient: userClient,
		logger:     logger,
	}
}

type ConversationResponse struct {
	ID          uuid.UUID        `json:"id"`
	Type        string           `json:"type"`
	Name        *string          `json:"name"`
	CreatedBy   uuid.UUID        `json:"created_by"`
	CreatedAt   string           `json:"created_at"`
	UpdatedAt   string           `json:"updated_at"`
	Members     []MemberResponse `json:"members"`
	LastMessage *MessageResponse `json:"last_message,omitempty"`
}

type MemberResponse struct {
	UserID      uuid.UUID `json:"user_id"`
	Role        string    `json:"role"`
	Username    string    `json:"username"`
	DisplayName *string   `json:"display_name"`
	AvatarURL   *string   `json:"avatar_url"`
}

// CreateDM creates or retrieves an existing DM conversation between two users.
func (s *ConversationService) CreateDM(ctx context.Context, creatorID, recipientID uuid.UUID) (*ConversationResponse, bool, error) {
	ctx, span := tracer.Start(ctx, "ConversationService.CreateDM")
	defer span.End()

	if creatorID == recipientID {
		return nil, false, fmt.Errorf("cannot create DM with yourself")
	}

	// Check if DM already exists
	existing, err := s.queries.FindDMConversation(ctx, sqlc.FindDMConversationParams{
		UserID:   creatorID,
		UserID_2: recipientID,
	})
	if err == nil {
		// DM exists — return it
		resp, err := s.enrichConversation(ctx, existing)
		if err != nil {
			return nil, false, err
		}
		return resp, false, nil // false = not newly created
	}

	// Create new DM in a transaction
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := s.queries.WithTx(tx)

	conv, err := qtx.CreateConversation(ctx, sqlc.CreateConversationParams{
		Type:      "dm",
		Name:      nil,
		CreatedBy: creatorID,
	})
	if err != nil {
		return nil, false, fmt.Errorf("create conversation: %w", err)
	}

	// Add both members
	_, err = qtx.AddConversationMember(ctx, sqlc.AddConversationMemberParams{
		ConversationID: conv.ID,
		UserID:         creatorID,
		Role:           "owner",
	})
	if err != nil {
		return nil, false, fmt.Errorf("add creator member: %w", err)
	}

	_, err = qtx.AddConversationMember(ctx, sqlc.AddConversationMemberParams{
		ConversationID: conv.ID,
		UserID:         recipientID,
		Role:           "member",
	})
	if err != nil {
		return nil, false, fmt.Errorf("add recipient member: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("commit tx: %w", err)
	}

	resp, err := s.enrichConversation(ctx, conv)
	if err != nil {
		return nil, false, err
	}

	return resp, true, nil // true = newly created
}

type ListParams struct {
	Limit  int32
	Offset int32
}

func (s *ConversationService) ListConversations(ctx context.Context, userID uuid.UUID, params ListParams) ([]ConversationResponse, int64, error) {
	ctx, span := tracer.Start(ctx, "ConversationService.ListConversations")
	defer span.End()

	conversations, err := s.queries.ListConversationsByUserID(ctx, sqlc.ListConversationsByUserIDParams{
		UserID: userID,
		Limit:  params.Limit,
		Offset: params.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list conversations: %w", err)
	}

	count, err := s.queries.CountConversationsByUserID(ctx, userID)
	if err != nil {
		return nil, 0, fmt.Errorf("count conversations: %w", err)
	}

	results := make([]ConversationResponse, 0, len(conversations))
	for _, conv := range conversations {
		resp, err := s.enrichConversation(ctx, conv)
		if err != nil {
			s.logger.Warn("failed to enrich conversation",
				zap.String("conversation_id", conv.ID.String()),
				zap.Error(err),
			)
			continue
		}
		results = append(results, *resp)
	}

	return results, count, nil
}

func (s *ConversationService) GetConversation(ctx context.Context, conversationID, userID uuid.UUID) (*ConversationResponse, error) {
	ctx, span := tracer.Start(ctx, "ConversationService.GetConversation")
	defer span.End()

	// Verify membership
	isMember, err := s.queries.IsConversationMember(ctx, sqlc.IsConversationMemberParams{
		ConversationID: conversationID,
		UserID:         userID,
	})
	if err != nil {
		return nil, fmt.Errorf("check membership: %w", err)
	}
	if !isMember {
		return nil, fmt.Errorf("not a member of this conversation")
	}

	conv, err := s.queries.GetConversationByID(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}

	return s.enrichConversation(ctx, conv)
}

// enrichConversation adds members with profile info and last message.
func (s *ConversationService) enrichConversation(ctx context.Context, conv sqlc.ChatSchemaConversation) (*ConversationResponse, error) {
	members, err := s.queries.GetConversationMembers(ctx, conv.ID)
	if err != nil {
		return nil, fmt.Errorf("get members: %w", err)
	}

	// Collect user IDs for batch profile fetch
	userIDs := make([]uuid.UUID, len(members))
	for i, m := range members {
		userIDs[i] = m.UserID
	}

	profiles, err := s.userClient.BatchGetProfiles(ctx, userIDs)
	if err != nil {
		s.logger.Warn("failed to fetch member profiles, using fallback",
			zap.Error(err),
		)
		// Graceful degradation — return members without profile data
		profiles = map[uuid.UUID]userclient.UserProfile{}
	}

	memberResponses := make([]MemberResponse, len(members))
	for i, m := range members {
		mr := MemberResponse{
			UserID: m.UserID,
			Role:   m.Role,
		}
		if p, ok := profiles[m.UserID]; ok {
			mr.Username = p.Username
			mr.DisplayName = p.DisplayName
			mr.AvatarURL = p.AvatarURL
		}
		memberResponses[i] = mr
	}

	resp := &ConversationResponse{
		ID:        conv.ID,
		Type:      conv.Type,
		Name:      conv.Name,
		CreatedBy: conv.CreatedBy,
		CreatedAt: conv.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt: conv.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		Members:   memberResponses,
	}

	// Attach last message
	lastMsg, err := s.queries.GetLastMessage(ctx, conv.ID)
	if err == nil {
		resp.LastMessage = &MessageResponse{
			ID:             lastMsg.ID,
			ConversationID: lastMsg.ConversationID,
			SenderID:       lastMsg.SenderID,
			Content:        lastMsg.Content,
			IsEdited:       lastMsg.IsEdited,
			CreatedAt:      lastMsg.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:      lastMsg.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	return resp, nil
}
