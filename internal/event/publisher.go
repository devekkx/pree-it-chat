package event

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

const (
	StreamName  = "CHAT_EVENTS"
	StreamSubjs = "chat.>"

	SubjectMessageCreated = "chat.message.created"
	SubjectMessageUpdated = "chat.message.updated"
	SubjectMessageDeleted = "chat.message.deleted"
)

var tracer = otel.Tracer("chat-event-publisher")

type Publisher struct {
	js     jetstream.JetStream
	logger *zap.Logger
}

type MessageEvent struct {
	MessageID      uuid.UUID `json:"message_id"`
	ConversationID uuid.UUID `json:"conversation_id"`
	SenderID       uuid.UUID `json:"sender_id"`
	Content        string    `json:"content,omitempty"`
	IsEdited       bool      `json:"is_edited"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func NewPublisher(nc *nats.Conn, logger *zap.Logger) (*Publisher, error) {
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("create jetstream context: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      StreamName,
		Subjects:  []string{StreamSubjs},
		Retention: jetstream.WorkQueuePolicy,
		MaxAge:    72 * time.Hour,
		Storage:   jetstream.FileStorage,
		Replicas:  1,
	})
	if err != nil {
		return nil, fmt.Errorf("create stream %s: %w", StreamName, err)
	}

	logger.Info("NATS JetStream stream ready", zap.String("stream", StreamName))

	return &Publisher{js: js, logger: logger}, nil
}

func (p *Publisher) Publish(ctx context.Context, subject string, event MessageEvent) {
	go func() {
		ctx, span := tracer.Start(ctx, "nats.publish."+subject)
		defer span.End()

		span.SetAttributes(
			attribute.String("nats.subject", subject),
			attribute.String("message.id", event.MessageID.String()),
			attribute.String("conversation.id", event.ConversationID.String()),
		)

		data, err := json.Marshal(event)
		if err != nil {
			p.logger.Error("failed to marshal event",
				zap.String("subject", subject),
				zap.Error(err),
			)
			return
		}

		pubCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		_, err = p.js.Publish(pubCtx, subject, data)
		if err != nil {
			p.logger.Error("failed to publish event",
				zap.String("subject", subject),
				zap.Error(err),
			)
			return
		}

		p.logger.Debug("event published",
			zap.String("subject", subject),
			zap.String("message_id", event.MessageID.String()),
		)
	}()
}
