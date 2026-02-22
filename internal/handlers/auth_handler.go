package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sentinel-app/sentinel/internal/services"
)

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	authService *services.AuthService
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(authService *services.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

type sendOTPRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type verifyOTPRequest struct {
	Email string `json:"email" binding:"required,email"`
	OTP   string `json:"otp" binding:"required,len=6"`
	Name  string `json:"name" binding:"required"`
}

// SendOTP handles POST /auth/otp/send
func (h *AuthHandler) SendOTP(c *gin.Context) {
	var req sendOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	if err := h.authService.SendOTP(c.Request.Context(), req.Email); err != nil {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "OTP sent"})
}

// VerifyOTP handles POST /auth/otp/verify
func (h *AuthHandler) VerifyOTP(c *gin.Context) {
	var req verifyOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	token, err := h.authService.VerifyOTP(c.Request.Context(), req.Email, req.OTP, req.Name)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": token})
}
