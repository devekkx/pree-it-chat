package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/devekkx/pree-it-chat/db"
	"github.com/devekkx/pree-it-chat/db/sqlc"
	"github.com/devekkx/pree-it-chat/internal/config"
	"github.com/devekkx/pree-it-chat/internal/event"
	"github.com/devekkx/pree-it-chat/internal/handler"
	"github.com/devekkx/pree-it-chat/internal/router"
	"github.com/devekkx/pree-it-chat/internal/service"
	"github.com/devekkx/pree-it-chat/internal/userclient"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	// --- OTel tracer ---
	ctx := context.Background()

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.OTelEndpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		logger.Fatal("failed to create otel exporter", zap.Error(err))
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(cfg.ServiceName),
			semconv.DeploymentEnvironmentKey.String(cfg.Environment),
		)),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	defer tp.Shutdown(ctx)

	// --- PostgreSQL ---
	poolCfg, err := pgxpool.ParseConfig(cfg.DBDSN())
	if err != nil {
		logger.Fatal("failed to parse DB DSN", zap.Error(err))
	}
	poolCfg.ConnConfig.Tracer = otelpgx.NewTracer()

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		logger.Fatal("failed to connect to DB", zap.Error(err))
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		logger.Fatal("failed to ping DB", zap.Error(err))
	}
	logger.Info("connected to PostgreSQL")

	// --- Migrations ---
	if err := db.RunMigrations(ctx, pool); err != nil {
		logger.Fatal("failed to run migrations", zap.Error(err))
	}
	logger.Info("migrations complete")

	// --- NATS ---
	var nc *nats.Conn
	for i := 0; i < 10; i++ {
		nc, err = nats.Connect(cfg.NatsURL)
		if err == nil {
			break
		}
		logger.Warn("NATS not ready, retrying...", zap.Int("attempt", i+1), zap.Error(err))
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		logger.Fatal("failed to connect to NATS", zap.Error(err))
	}
	defer nc.Close()
	logger.Info("connected to NATS")

	// --- Event publisher ---
	publisher, err := event.NewPublisher(nc, logger)
	if err != nil {
		logger.Fatal("failed to create event publisher", zap.Error(err))
	}

	// --- Services ---
	queries := sqlc.New(pool)
	userClient := userclient.New(cfg.UserServiceURL)
	convService := service.NewConversationService(pool, queries, userClient, logger)
	msgService := service.NewMessageService(queries, publisher, logger)

	// --- Handlers ---
	convHandler := handler.NewConversationHandler(convService, logger)
	msgHandler := handler.NewMessageHandler(msgService, logger)

	// --- HTTP server ---
	r := router.Setup(convHandler, msgHandler)
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("starting chat-service", zap.Int("port", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server error", zap.Error(err))
		}
	}()

	// --- Graceful shutdown ---
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("shutting down chat-service...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown error", zap.Error(err))
	}

	logger.Info("chat-service stopped")
}
