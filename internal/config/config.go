package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration for both the API and worker binaries.
type Config struct {
	// HTTP
	HTTPPort string

	// PostgreSQL
	PostgresDSN string

	// RabbitMQ
	RabbitMQURL string

	// Retry policy
	MaxAttempts  int
	RetryDelayMS int

	// Email (SMTP)
	SMTPHost string
	SMTPPort string
	SMTPUser string
	SMTPPass string
	SMTPFrom string

	// SMS (generic HTTP provider, e.g. Twilio-like)
	SMSProviderURL   string
	SMSProviderSID   string
	SMSProviderToken string
	SMSFrom          string

	// Push (FCM HTTP v1 / legacy server key)
	FCMServerKey string
	FCMEndpoint  string
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

// Load reads a .env file if present (ignored if missing) and returns a populated Config.
func Load() *Config {
	_ = godotenv.Load()

	return &Config{
		HTTPPort: getEnv("HTTP_PORT", "8080"),

		PostgresDSN: getEnv("POSTGRES_DSN",
			"postgres://notifier:notifier@localhost:5432/notifications?sslmode=disable"),

		RabbitMQURL: getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),

		MaxAttempts:  getEnvInt("MAX_ATTEMPTS", 3),
		RetryDelayMS: getEnvInt("RETRY_DELAY_MS", 5000),

		SMTPHost: getEnv("SMTP_HOST", "localhost"),
		SMTPPort: getEnv("SMTP_PORT", "1025"),
		SMTPUser: getEnv("SMTP_USER", ""),
		SMTPPass: getEnv("SMTP_PASS", ""),
		SMTPFrom: getEnv("SMTP_FROM", "no-reply@example.com"),

		SMSProviderURL:   getEnv("SMS_PROVIDER_URL", ""),
		SMSProviderSID:   getEnv("SMS_PROVIDER_SID", ""),
		SMSProviderToken: getEnv("SMS_PROVIDER_TOKEN", ""),
		SMSFrom:          getEnv("SMS_FROM", ""),

		FCMServerKey: getEnv("FCM_SERVER_KEY", ""),
		FCMEndpoint:  getEnv("FCM_ENDPOINT", "https://fcm.googleapis.com/fcm/send"),
	}
}
