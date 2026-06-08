package handler

import (
	"net/http"
	"strconv"

	"github.com/devekkx/pree-it-chat/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type ConversationHandler struct {
	convService *service.ConversationService
	logger      *zap.Logger
}

func NewConversationHandler(convService *service.ConversationService, logger *zap.Logger) *ConversationHandler {
	return &ConversationHandler{
		convService: convService,
		logger:      logger,
	}
}

func getUserID(c *gin.Context) (uuid.UUID, bool) {
	raw := c.GetHeader("X-User-ID")
	if raw == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing X-User-ID header"})
		return uuid.Nil, false
	}

	id, err := uuid.Parse(raw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid X-User-ID"})
		return uuid.Nil, false
	}

	return id, true
}

type createDMRequest struct {
	RecipientID uuid.UUID `json:"recipient_id" binding:"required"`
}

func (h *ConversationHandler) CreateDM(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	var req createDMRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "recipient_id is required"})
		return
	}

	conv, created, err := h.convService.CreateDM(c.Request.Context(), userID, req.RecipientID)
	if err != nil {
		h.logger.Error("create DM failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}

	c.JSON(status, conv)
}

func (h *ConversationHandler) ListConversations(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	if limit < 1 || limit > 50 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	conversations, total, err := h.convService.ListConversations(c.Request.Context(), userID, service.ListParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		h.logger.Error("list conversations failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list conversations"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"conversations": conversations,
		"total":         total,
		"limit":         limit,
		"offset":        offset,
	})
}

func (h *ConversationHandler) GetConversation(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	convID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid conversation ID"})
		return
	}

	conv, err := h.convService.GetConversation(c.Request.Context(), convID, userID)
	if err != nil {
		h.logger.Error("get conversation failed", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, conv)
}
