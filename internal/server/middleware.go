package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/branchd-dev/branchd/internal/auth"
	"github.com/branchd-dev/branchd/internal/models"
)

const (
	bearerPrefix = "Bearer "
)

var (
	ErrMissingAuthHeader  = errors.New("missing authorization header")
	ErrInvalidAuthFormat  = errors.New("invalid authorization header format")
	ErrEmptyToken         = errors.New("empty token")
	ErrInvalidToken       = errors.New("invalid token")
	ErrUserNotFound       = errors.New("user not found")
)

func setSession(c *gin.Context, sessionData *auth.SessionData) {
	c.Set("session", sessionData)
}

func GetSessionData(c *gin.Context) (*auth.SessionData, bool) {
	session, exists := c.Get("session")
	if !exists {
		return nil, false
	}

	sessionData, ok := session.(*auth.SessionData)
	return sessionData, ok
}

func extractBearerToken(authHeader string) (string, error) {
	if authHeader == "" {
		return "", ErrMissingAuthHeader
	}

	if !strings.HasPrefix(authHeader, bearerPrefix) {
		return "", ErrInvalidAuthFormat
	}

	token := strings.TrimPrefix(authHeader, bearerPrefix)
	if token == "" {
		return "", ErrEmptyToken
	}

	return token, nil
}

func respondWithError(c *gin.Context, log zerolog.Logger, statusCode int, err error, message string) {
	log.Warn().Err(err).Msg(message)
	c.JSON(statusCode, gin.H{"error": message})
	c.Abort()
}

// JWTAuthMiddleware validates JWT tokens for both web and CLI
func JWTAuthMiddleware(db *gorm.DB, log zerolog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract token from Authorization header
		authHeader := c.GetHeader("Authorization")
		token, err := extractBearerToken(authHeader)
		if err != nil {
			var message string
			switch err {
			case ErrMissingAuthHeader:
				message = "Missing authorization header"
			case ErrInvalidAuthFormat:
				message = "Invalid authorization header format"
			case ErrEmptyToken:
				message = "Empty token"
			}
			respondWithError(c, log, http.StatusUnauthorized, err, message)
			return
		}

		// Validate JWT token
		claims, err := auth.ValidateToken(token)
		if err != nil {
			log.Error().Err(err).Msg("Failed to validate JWT token")
			respondWithError(c, log, http.StatusUnauthorized, ErrInvalidToken, "Invalid or expired token")
			return
		}

		// Verify user exists in database
		var user models.User
		if err := db.Where("id = ?", claims.UserID).First(&user).Error; err != nil {
			log.Error().Err(err).Str("user_id", claims.UserID).Msg("User not found")
			respondWithError(c, log, http.StatusUnauthorized, ErrUserNotFound, "User not found")
			return
		}

		// Set session data
		sessionData := &auth.SessionData{
			UserID:     user.ID,
			Email:      user.Email,
			IsAdmin:    user.IsAdmin,
			AuthMethod: "jwt", // Can be differentiated by endpoint if needed
		}
		setSession(c, sessionData)

		c.Next()
	}
}

// AdminOnlyMiddleware ensures the authenticated user is an admin
func AdminOnlyMiddleware(log zerolog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionData, exists := GetSessionData(c)
		if !exists {
			respondWithError(c, log, http.StatusUnauthorized, errors.New("no session"), "Unauthorized")
			return
		}

		if !sessionData.IsAdmin {
			respondWithError(c, log, http.StatusForbidden, errors.New("not admin"), "Admin access required")
			return
		}

		c.Next()
	}
}
