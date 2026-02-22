package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	Port    string
	GinMode string

	// Database
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string

	// Redis
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// JWT
	JWTSecret string

	// AWS
	AWSRegion  string
	AWSProfile string
	S3Bucket   string

	// SES
	SESFromOTP   string
	SESFromAlert string

	// OTP
	OTPLength        int
	OTPExpiryMinutes int

	// Templates
	TemplatesDir string

	// File Upload
	MaxFileSizeMB int
}

// Load reads configuration from environment variables with sensible defaults.
// Validates that all critical production values are set.
func Load() *Config {
	cfg := &Config{
		Port:    getEnv("PORT", "8080"),
		GinMode: getEnv("GIN_MODE", "debug"),

		DBHost:     requireEnv("DB_HOST"),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBUser:     requireEnv("DB_USER"),
		DBPassword: requireEnv("DB_PASSWORD"),
		DBName:     requireEnv("DB_NAME"),
		DBSSLMode:  getEnv("DB_SSLMODE", "require"),

		RedisAddr:     requireEnv("REDIS_ADDR"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RedisDB:       getEnvInt("REDIS_DB", 0),

		JWTSecret: requireEnv("JWT_SECRET"),

		AWSRegion:  requireEnv("AWS_REGION"),
		AWSProfile: getEnv("AWS_PROFILE", ""),
		S3Bucket:   requireEnv("S3_BUCKET"),

		SESFromOTP:   requireEnv("SES_FROM_OTP"),
		SESFromAlert: requireEnv("SES_FROM_ALERT"),

		OTPLength:        getEnvInt("OTP_LENGTH", 6),
		OTPExpiryMinutes: getEnvInt("OTP_EXPIRY_MINUTES", 5),

		TemplatesDir: getEnv("TEMPLATES_DIR", "internal/templates/html"),

		MaxFileSizeMB: getEnvInt("MAX_FILE_SIZE_MB", 10),
	}

	return cfg
}

// DSN returns the PostgreSQL connection string.
func (c *Config) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName, c.DBSSLMode,
	)
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

// requireEnv loads an environment variable and fatals if it is empty.
// This ensures the server cannot start in production with missing config.
func requireEnv(key string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		log.Fatalf("FATAL: required environment variable %s is not set", key)
	}
	return v
}
