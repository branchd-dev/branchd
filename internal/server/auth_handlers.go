package server

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/branchd-dev/branchd/internal/auth"
	"github.com/branchd-dev/branchd/internal/models"
)

// SetupRequest represents the first-run setup request
type SetupRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
	Name     string `json:"name" binding:"required"`
}

// LoginRequest represents a login request
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse represents a login response
type LoginResponse struct {
	Token string      `json:"token"`
	User  *UserDetail `json:"user"`
}

// UserDetail represents user information returned in responses
type UserDetail struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	IsAdmin   bool      `json:"is_admin"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateUserRequest represents a request to create a new user
type CreateUserRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Name     string `json:"name" binding:"required"`
	Password string `json:"password" binding:"required"`
	IsAdmin  bool   `json:"is_admin"`
}

// CreateUserResponse includes the created user details
type CreateUserResponse struct {
	User *UserDetail `json:"user"`
}

// @Summary First-run setup
// @Description Creates the first admin user (only works if no users exist)
// @Tags auth
// @Accept json
// @Produce json
// @Param request body SetupRequest true "Setup request"
// @Success 200 {object} LoginResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 409 {object} map[string]interface{}
// @Router /api/setup [post]
func (s *Server) setupFirstAdmin(c *gin.Context) {
	var req SetupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if any users exist
	var count int64
	if err := s.db.Model(&models.User{}).Count(&count).Error; err != nil {
		s.logger.Error().Err(err).Msg("Failed to count users")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "Setup already completed"})
		return
	}

	// Generate JWT secret (64 hex characters = 32 bytes of randomness)
	jwtSecretBytes := make([]byte, 32)
	if _, err := rand.Read(jwtSecretBytes); err != nil {
		s.logger.Error().Err(err).Msg("Failed to generate JWT secret")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to initialize system"})
		return
	}
	jwtSecret := hex.EncodeToString(jwtSecretBytes)

	// Create Config singleton with JWT secret
	config := &models.Config{
		JWTSecret:   jwtSecret,
		MaxRestores: 5, // Default to keeping 5 restores
		// These will be set later during onboarding
		ConnectionString: "",
		PostgresVersion:  "",
	}
	if err := s.db.Create(config).Error; err != nil {
		s.logger.Error().Err(err).Msg("Failed to create config")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to initialize system"})
		return
	}

	// Initialize JWT authentication with the generated secret
	auth.InitializeJWT(jwtSecret)

	// Hash password
	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to hash password")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}

	// Create admin user
	user := &models.User{
		Email:        req.Email,
		PasswordHash: passwordHash,
		Name:         req.Name,
		IsAdmin:      true,
	}

	if err := s.db.Create(user).Error; err != nil {
		s.logger.Error().Err(err).Msg("Failed to create admin user")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}

	// Generate JWT token
	token, err := auth.GenerateToken(user.ID, user.Email, user.IsAdmin)
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to generate token")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	s.logger.Info().Str("user_id", user.ID).Str("email", user.Email).Msg("First admin user created")

	c.JSON(http.StatusOK, LoginResponse{
		Token: token,
		User: &UserDetail{
			ID:        user.ID,
			Email:     user.Email,
			Name:      user.Name,
			IsAdmin:   user.IsAdmin,
			CreatedAt: user.CreatedAt,
		},
	})
}

// @Summary Login
// @Description Authenticate with email and password
// @Tags auth
// @Accept json
// @Produce json
// @Param request body LoginRequest true "Login request"
// @Success 200 {object} LoginResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /api/auth/login [post]
func (s *Server) login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Find user by email
	var user models.User
	if err := s.db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
			return
		}
		s.logger.Error().Err(err).Msg("Failed to find user")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	// Verify password
	if err := auth.VerifyPassword(req.Password, user.PasswordHash); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		return
	}

	// Generate JWT token
	token, err := auth.GenerateToken(user.ID, user.Email, user.IsAdmin)
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to generate token")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	s.logger.Info().Str("user_id", user.ID).Str("email", user.Email).Msg("User logged in")

	c.JSON(http.StatusOK, LoginResponse{
		Token: token,
		User: &UserDetail{
			ID:        user.ID,
			Email:     user.Email,
			Name:      user.Name,
			IsAdmin:   user.IsAdmin,
			CreatedAt: user.CreatedAt,
		},
	})
}

// @Summary Get current user
// @Description Get information about the currently authenticated user
// @Tags auth
// @Produce json
// @Security BearerAuth
// @Success 200 {object} UserDetail
// @Failure 401 {object} map[string]interface{}
// @Router /api/auth/me [get]
func (s *Server) getCurrentUser(c *gin.Context) {
	sessionData, exists := GetSessionData(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var user models.User
	if err := s.db.Where("id = ?", sessionData.UserID).First(&user).Error; err != nil {
		s.logger.Error().Err(err).Str("user_id", sessionData.UserID).Msg("Failed to find user")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	c.JSON(http.StatusOK, UserDetail{
		ID:        user.ID,
		Email:     user.Email,
		Name:      user.Name,
		IsAdmin:   user.IsAdmin,
		CreatedAt: user.CreatedAt,
	})
}

// @Summary List users
// @Description List all users (admin only)
// @Tags users
// @Produce json
// @Security BearerAuth
// @Success 200 {array} UserDetail
// @Failure 401 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Router /api/users [get]
func (s *Server) listUsers(c *gin.Context) {
	var users []models.User
	if err := s.db.Order("created_at DESC").Find(&users).Error; err != nil {
		s.logger.Error().Err(err).Msg("Failed to list users")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	userDetails := make([]UserDetail, len(users))
	for i, user := range users {
		userDetails[i] = UserDetail{
			ID:        user.ID,
			Email:     user.Email,
			Name:      user.Name,
			IsAdmin:   user.IsAdmin,
			CreatedAt: user.CreatedAt,
		}
	}

	c.JSON(http.StatusOK, userDetails)
}

// @Summary Create user
// @Description Create a new user (admin only)
// @Tags users
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateUserRequest true "Create user request"
// @Success 201 {object} CreateUserResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Router /api/users [post]
func (s *Server) createUser(c *gin.Context) {
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Hash the provided password
	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to hash password")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}

	// Create user
	user := &models.User{
		Email:        req.Email,
		PasswordHash: passwordHash,
		Name:         req.Name,
		IsAdmin:      req.IsAdmin,
	}

	if err := s.db.Create(user).Error; err != nil {
		s.logger.Error().Err(err).Msg("Failed to create user")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}

	sessionData, _ := GetSessionData(c)
	s.logger.Info().
		Str("user_id", user.ID).
		Str("email", user.Email).
		Str("created_by", sessionData.UserID).
		Msg("User created")

	c.JSON(http.StatusCreated, CreateUserResponse{
		User: &UserDetail{
			ID:        user.ID,
			Email:     user.Email,
			Name:      user.Name,
			IsAdmin:   user.IsAdmin,
			CreatedAt: user.CreatedAt,
		},
	})
}

// @Summary Delete user
// @Description Delete a user (admin only, cannot delete self)
// @Tags users
// @Produce json
// @Security BearerAuth
// @Param id path string true "User ID"
// @Success 204
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/users/{id} [delete]
func (s *Server) deleteUser(c *gin.Context) {
	userID := c.Param("id")

	sessionData, _ := GetSessionData(c)

	// Prevent deleting self
	if userID == sessionData.UserID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot delete yourself"})
		return
	}

	// Find user
	var user models.User
	if err := s.db.Where("id = ?", userID).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		s.logger.Error().Err(err).Msg("Failed to find user")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	// Delete user
	if err := s.db.Delete(&user).Error; err != nil {
		s.logger.Error().Err(err).Msg("Failed to delete user")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete user"})
		return
	}

	s.logger.Info().
		Str("user_id", userID).
		Str("deleted_by", sessionData.UserID).
		Msg("User deleted")

	c.Status(http.StatusNoContent)
}
