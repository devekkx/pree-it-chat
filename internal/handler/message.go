package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/devekkx/pree-it-chat/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type MessageHandler struct {
	msgService *service.MessageService
	logger     *zap.Logger
}

func NewMessageHandler(msgService *service.MessageService, logger *zap.Logger) *MessageHandler {
	return &MessageHandler{
		msgService: msgService,
		logger:     logger,
	}
}

type sendMessageRequest struct {
	Content string `json:"content" binding:"required"`
}

func (h *MessageHandler) SendMessage(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	convID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid conversation ID"})
		return
	}

	var req sendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content is required"})
		return
	}

	msg, err := h.msgService.SendMessage(c.Request.Context(), convID, userID, req.Content)
	if err != nil {
		h.logger.Error("send message failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, msg)
}

func (h *MessageHandler) ListMessages(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	convID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid conversation ID"})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "30"))
	if limit < 1 || limit > 100 {
		limit = 30
	}

	params := service.ListMessagesParams{
		ConversationID: convID,
		Limit:          int32(limit),
	}

	// Cursor-based pagination
	if cursorStr := c.Query("cursor"); cursorStr != "" {
		t, err := time.Parse(time.RFC3339Nano, cursorStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cursor format, use RFC3339Nano"})
			return
		}
		params.Cursor = &t
	}

	messages, err := h.msgService.ListMessages(c.Request.Context(), userID, params)
	if err != nil {
		h.logger.Error("list messages failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Next cursor = created_at of the last message in the result set
	var nextCursor *string
	if len(messages) == int(params.Limit) {
		last := messages[len(messages)-1].CreatedAt
		nextCursor = &last
	}

	c.JSON(http.StatusOK, gin.H{
		"messages":    messages,
		"next_cursor": nextCursor,
	})
}

type editMessageRequest struct {
	Content string `json:"content" binding:"required"`
}

func (h *MessageHandler) EditMessage(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	msgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message ID"})
		return
	}

	var req editMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content is required"})
		return
	}

	msg, err := h.msgService.EditMessage(c.Request.Context(), msgID, userID, req.Content)
	if err != nil {
		h.logger.Error("edit message failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, msg)
}

func (h *MessageHandler) DeleteMessage(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	msgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message ID"})
		return
	}

	if err := h.msgService.DeleteMessage(c.Request.Context(), msgID, userID); err != nil {
		h.logger.Error("delete message failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "message deleted"})
}
