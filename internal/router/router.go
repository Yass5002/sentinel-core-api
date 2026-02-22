package router

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/sentinel-app/sentinel/internal/handlers"
	"github.com/sentinel-app/sentinel/internal/middleware"
	"github.com/sentinel-app/sentinel/internal/services"
	"gorm.io/gorm"
)

// Setup configures all routes, CORS, trusted proxies, and middleware.
func Setup(
	r *gin.Engine,
	authHandler *handlers.AuthHandler,
	eventHandler *handlers.EventHandler,
	locationHandler *handlers.LocationHandler,
	fileHandler *handlers.FileHandler,
	profileHandler *handlers.ProfileHandler,
	contactHandler *handlers.ContactHandler,
	authService *services.AuthService,
	db *gorm.DB,
	rdb *redis.Client,
) {
	// Trust Cloudflare proxy IP ranges so RemoteIP() returns the real client IP.
	// These are Cloudflare's published IPv4 ranges as of 2026-02.
	// See: https://www.cloudflare.com/ips-v4/
	r.SetTrustedProxies([]string{
		"173.245.48.0/20",
		"103.21.244.0/22",
		"103.22.200.0/22",
		"103.31.4.0/22",
		"141.101.64.0/18",
		"108.162.192.0/18",
		"190.93.240.0/20",
		"188.114.96.0/20",
		"197.234.240.0/22",
		"198.41.128.0/17",
		"162.158.0.0/15",
		"104.16.0.0/13",
		"104.24.0.0/14",
		"172.64.0.0/13",
		"131.0.72.0/22",
	})

	// CORS — allow your frontend origins
	r.Use(cors.New(cors.Config{
		AllowOrigins: []string{
			"https://sentinel.yssn.tech",
			"https://app.sentinel.yssn.tech",
			"http://localhost:3000", // local dev
		},
		AllowMethods:     []string{"GET", "POST", "PATCH", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Deep health check — verifies DB and Redis connectivity
	r.GET("/health", func(c *gin.Context) {
		sqlDB, err := db.DB()
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "error", "detail": "database pool unavailable"})
			return
		}
		if err := sqlDB.Ping(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "error", "detail": "database unreachable"})
			return
		}
		if err := rdb.Ping(context.Background()).Err(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "error", "detail": "redis unreachable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Auth routes (public)
	auth := r.Group("/auth")
	{
		auth.POST("/otp/send", authHandler.SendOTP)
		auth.POST("/otp/verify", authHandler.VerifyOTP)
	}

	// Protected routes
	api := r.Group("/")
	api.Use(middleware.AuthMiddleware(authService))
	{
		// Events
		api.POST("/events", eventHandler.CreateEvent)
		api.GET("/events", eventHandler.ListEvents)
		api.GET("/events/:id", eventHandler.GetEvent)
		api.POST("/events/:id/checkin", eventHandler.CheckIn)
		api.DELETE("/events/:id", eventHandler.CancelEvent)

		// File uploads (staged — upload before creating event)
		api.POST("/files", fileHandler.Upload)
		api.GET("/files", fileHandler.ListStaged)
		api.DELETE("/files/:id", fileHandler.DeleteStaged)

		// Location
		api.POST("/location", locationHandler.UpdateLocation)
		api.GET("/location", locationHandler.GetLocation)

		// Profile
		api.GET("/profile", profileHandler.GetProfile)
		api.PATCH("/profile", profileHandler.UpdateProfile)

		// Saved Contacts
		api.GET("/contacts", contactHandler.ListContacts)
		api.POST("/contacts", contactHandler.CreateContact)
		api.PATCH("/contacts/:id", contactHandler.UpdateContact)
		api.DELETE("/contacts/:id", contactHandler.DeleteContact)
	}
}
