package router

import (
	"github.com/devekkx/pree-it-chat/internal/handler"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

func Setup(
	convHandler *handler.ConversationHandler,
	msgHandler *handler.MessageHandler,
) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(otelgin.Middleware("chat-service"))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	v1 := r.Group("/api/v1/chat")
	{
		conversations := v1.Group("/conversations")
		{
			conversations.POST("", convHandler.CreateDM)
			conversations.GET("", convHandler.ListConversations)
			conversations.GET("/:id", convHandler.GetConversation)
			conversations.GET("/:id/messages", msgHandler.ListMessages)
			conversations.POST("/:id/messages", msgHandler.SendMessage)
		}

		messages := v1.Group("/messages")
		{
			messages.PATCH("/:id", msgHandler.EditMessage)
			messages.DELETE("/:id", msgHandler.DeleteMessage)
		}
	}

	return r
}
