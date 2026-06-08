package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/devekkx/pree-it-chat/pkg/secrets"
)

type Config struct {
	ServiceName    string
	Port           int
	Environment    string
	DBHost         string
	DBPort         int
	DBUser         string
	DBPassword     string
	DBName         string
	DBSSLMode      string
	NatsURL        string
	OTelEndpoint   string
	UserServiceURL string
}

func Load() (*Config, error) {
	port, err := strconv.Atoi(getEnvOrDefault("CHAT_SERVICE_PORT", "8084"))
	if err != nil {
		return nil, fmt.Errorf("invalid CHAT_SERVICE_PORT: %w", err)
	}

	dbPort, err := strconv.Atoi(getEnvOrDefault("CHAT_DB_PORT", "5432"))
	if err != nil {
		return nil, fmt.Errorf("invalid CHAT_DB_PORT: %w", err)
	}

	dbPassword := secrets.Get("CHAT_DB_PASSWORD", "chat_db_password")
	if dbPassword == "" {
		return nil, fmt.Errorf("chat DB password not found in env or secrets")
	}

	return &Config{
		ServiceName:    getEnvOrDefault("CHAT_SERVICE_NAME", "chat-service"),
		Port:           port,
		Environment:    getEnvOrDefault("CHAT_ENVIRONMENT", "production"),
		DBHost:         getEnvOrDefault("CHAT_DB_HOST", "postgres"),
		DBPort:         dbPort,
		DBUser:         getEnvOrDefault("CHAT_DB_USER", "chat_service"),
		DBPassword:     dbPassword,
		DBName:         getEnvOrDefault("CHAT_DB_NAME", "preeit"),
		DBSSLMode:      getEnvOrDefault("CHAT_DB_SSLMODE", "disable"),
		NatsURL:        getEnvOrDefault("CHAT_NATS_URL", "nats://nats:4222"),
		OTelEndpoint:   getEnvOrDefault("CHAT_OTEL_ENDPOINT", "otel-collector:4317"),
		UserServiceURL: getEnvOrDefault("CHAT_USER_SERVICE_URL", "http://user-service:8083"),
	}, nil
}

func (c *Config) DBDSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName, c.DBSSLMode,
	)
}

func getEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
