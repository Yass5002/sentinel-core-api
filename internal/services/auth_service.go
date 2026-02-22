package services

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/sentinel-app/sentinel/internal/config"
	"github.com/sentinel-app/sentinel/internal/models"
	"github.com/sentinel-app/sentinel/internal/templates"
	"gorm.io/gorm"
)

// AuthService handles OTP generation/verification and JWT token management.
type AuthService struct {
	db    *gorm.DB
	rdb   *redis.Client
	cfg   *config.Config
	email *EmailService
	tmpl  *templates.Engine
}

// NewAuthService creates a new AuthService.
func NewAuthService(db *gorm.DB, rdb *redis.Client, cfg *config.Config, email *EmailService, tmpl *templates.Engine) *AuthService {
	return &AuthService{db: db, rdb: rdb, cfg: cfg, email: email, tmpl: tmpl}
}

// SendOTP generates a random OTP, stores it in Redis with expiry, and emails it.
func (s *AuthService) SendOTP(ctx context.Context, emailAddr string) error {
	// Rate limiting: max 1 OTP per 60 seconds per email
	rateLimitKey := fmt.Sprintf("otp_rate:%s", emailAddr)
	if exists, _ := s.rdb.Exists(ctx, rateLimitKey).Result(); exists > 0 {
		return fmt.Errorf("please wait before requesting another OTP")
	}

	// Generate OTP
	otp, err := generateOTP(s.cfg.OTPLength)
	if err != nil {
		return fmt.Errorf("failed to generate OTP: %w", err)
	}

	// Store OTP in Redis with expiry
	otpKey := fmt.Sprintf("otp:%s", emailAddr)
	expiry := time.Duration(s.cfg.OTPExpiryMinutes) * time.Minute
	if err := s.rdb.Set(ctx, otpKey, otp, expiry).Err(); err != nil {
		return fmt.Errorf("failed to store OTP: %w", err)
	}

	// Set rate limit (60 seconds)
	if err := s.rdb.Set(ctx, rateLimitKey, "1", 60*time.Second).Err(); err != nil {
		log.Printf("Warning: failed to set OTP rate limit key: %v", err)
	}

	// Send OTP email
	subject := "Sentinel — Your verification code"
	body, err := s.tmpl.RenderOTP(templates.OTPData{
		Code:          otp,
		ExpiryMinutes: s.cfg.OTPExpiryMinutes,
	})
	if err != nil {
		return fmt.Errorf("failed to render OTP email: %w", err)
	}

	if err := s.email.SendOTP(ctx, emailAddr, subject, body); err != nil {
		return fmt.Errorf("failed to send OTP email: %w", err)
	}

	return nil
}

// VerifyOTP checks the OTP against Redis and returns a JWT if valid.
func (s *AuthService) VerifyOTP(ctx context.Context, emailAddr, otp, name string) (string, error) {
	otpKey := fmt.Sprintf("otp:%s", emailAddr)

	storedOTP, err := s.rdb.Get(ctx, otpKey).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("OTP expired or not found")
	}
	if err != nil {
		return "", fmt.Errorf("failed to verify OTP: %w", err)
	}

	if storedOTP != otp {
		return "", fmt.Errorf("invalid OTP")
	}

	// Delete OTP after successful verification
	s.rdb.Del(ctx, otpKey)

	// Find or create user
	var entity models.Entity
	result := s.db.Where("signal = ?", emailAddr).First(&entity)
	if result.Error == gorm.ErrRecordNotFound {
		entity = models.Entity{
			Signal: emailAddr,
			Alias:  name,
		}
		if err := s.db.Create(&entity).Error; err != nil {
			return "", fmt.Errorf("failed to create user: %w", err)
		}
	} else if result.Error != nil {
		return "", fmt.Errorf("database error: %w", result.Error)
	} else if entity.Alias == "" && name != "" {
		// Backfill name for existing users who don't have one
		entity.Alias = name
		s.db.Save(&entity)
	}

	// Generate JWT
	token, err := s.generateJWT(entity.ID)
	if err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}

	return token, nil
}

// ParseToken validates a JWT and returns the user ID.
func (s *AuthService) ParseToken(tokenString string) (uuid.UUID, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.cfg.JWTSecret), nil
	})
	if err != nil {
		return uuid.Nil, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return uuid.Nil, fmt.Errorf("invalid token")
	}

	sub, ok := claims["sub"].(string)
	if !ok {
		return uuid.Nil, fmt.Errorf("invalid token claims")
	}

	userID, err := uuid.Parse(sub)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid user ID in token")
	}

	return userID, nil
}

func (s *AuthService) generateJWT(userID uuid.UUID) (string, error) {
	claims := jwt.MapClaims{
		"sub": userID.String(),
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(30 * 24 * time.Hour).Unix(), // 30 days
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.cfg.JWTSecret))
}

// generateOTP creates a cryptographically secure numeric OTP.
func generateOTP(length int) (string, error) {
	otp := ""
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		otp += fmt.Sprintf("%d", n.Int64())
	}
	return otp, nil
}
