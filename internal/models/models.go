package models

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

// BaseModel provides common fields and auto-generated ULID for all models
type BaseModel struct {
	ID        string    `json:"id" gorm:"primaryKey;type:varchar(26)"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
}

// BeforeCreate generates a ULID for the ID field if it's empty
func (b *BaseModel) BeforeCreate(tx *gorm.DB) error {
	if b.ID == "" {
		b.ID = ulid.Make().String()
	}
	return nil
}

// Config represents the global configuration for the single-tenant deployment
// This is a singleton model (only one row should exist)
type Config struct {
	BaseModel
	// Authentication configuration
	JWTSecret string `json:"-" gorm:"type:varchar(64);not null"` // Auto-generated on first setup (64 hex chars)

	// Database source configuration
	ConnectionString string `json:"connection_string" gorm:"type:text"` // PostgreSQL connection string
	PostgresVersion  string `json:"postgres_version"`
	SchemaOnly       bool   `json:"schema_only" gorm:"not null;default:true"` // If true, only restore schema (no data)

	// PostgreSQL configuration for branches
	BranchPostgresqlConf string `json:"branch_postgresql_conf" gorm:"type:text"`

	// Refresh configuration (for periodic pg_dump/restore)
	RefreshSchedule string     `json:"refresh_schedule"`  // Cron expression, e.g. "0 2 * * *" (2am daily), empty = no auto refresh
	LastRefreshedAt *time.Time `json:"last_refreshed_at"` // When was last refresh completed
	NextRefreshAt   *time.Time `json:"next_refresh_at"`   // Calculated from cron schedule

	// Storage management
	MaxRestores int `json:"max_restores" gorm:"not null;default:1"` // Maximum number of restores to keep (restores with branches are excluded from cleanup)

	// TLS/Domain configuration (optional - for Let's Encrypt)
	Domain           string `json:"domain"`             // Custom domain (e.g. "db.company.com"), empty = use self-signed cert
	LetsEncryptEmail string `json:"lets_encrypt_email"` // Email for Let's Encrypt ACME, required if Domain is set

	// Computed fields (populated at runtime, not persisted)
	DatabaseName string `json:"database_name" gorm:"-"` // Extracted from ConnectionString
}

// AfterFind populates computed fields after loading from database
func (c *Config) AfterFind(tx *gorm.DB) error {
	// Populate computed fields
	c.DatabaseName = c.databaseName()
	return nil
}

// databaseName extracts the database name from the PostgreSQL connection string
func (c *Config) databaseName() string {
	connStr := c.ConnectionString

	// Try parsing as URL first (postgresql://user:pass@host:port/dbname)
	if strings.HasPrefix(connStr, "postgresql://") || strings.HasPrefix(connStr, "postgres://") {
		u, err := url.Parse(connStr)
		if err == nil && len(u.Path) > 1 {
			return strings.TrimPrefix(u.Path, "/")
		}
	}

	// Fallback: look for dbname= in key-value format
	parts := strings.SplitSeq(connStr, " ")
	for part := range parts {
		if after, ok := strings.CutPrefix(part, "dbname="); ok {
			return after
		}
	}

	// Default fallback
	return "postgres"
}

// Restore represents a PostgreSQL database restore that branches are created from
// Each restore is a snapshot from pg_dump/restore with a UTC datetime-based name
type Restore struct {
	BaseModel
	Name        string     `json:"name" gorm:"not null;unique"` // restore_YYYYMMDDHHmmss format (e.g., restore_20251017143202)
	SchemaOnly  bool       `json:"schema_only" gorm:"not null;default:false"`
	SchemaReady bool       `json:"schema_ready" gorm:"not null;default:false"`
	DataReady   bool       `json:"data_ready" gorm:"not null;default:false"`
	ReadyAt     *time.Time `json:"ready_at"` // When restore became ready for branching
	Port        int        `json:"port" gorm:"not null"`

	// Relationships
	Branches []Branch `json:"branches,omitempty" gorm:"foreignKey:RestoreID"`
}

// GenerateRestoreName generates a restore name with UTC datetime format
// Returns: restore_YYYYMMDDHHmmss (e.g., restore_20251017143202)
func GenerateRestoreName() string {
	return fmt.Sprintf("restore_%s", time.Now().UTC().Format("20060102150405"))
}

// Branch represents a database branch (ZFS clone) within a cluster
type Branch struct {
	BaseModel
	Name        string `json:"name" gorm:"not null"`
	RestoreID   string `json:"restore_id" gorm:"not null"`
	CreatedByID string `json:"created_by_id" gorm:"not null"`
	User        string `json:"user" gorm:"not null"`           // 16-char URL-safe random string (encrypted)
	Password    string `json:"password" gorm:"not null"`       // 32-char URL-safe random string (encrypted)
	Port        int    `json:"port" gorm:"not null;default:0"` // Set after successful creation

	// Relationships
	Restore   Restore `json:"restore,omitzero" gorm:"foreignKey:RestoreID;constraint:OnDelete:CASCADE"`
	CreatedBy *User   `json:"created_by,omitempty" gorm:"foreignKey:CreatedByID;references:ID;constraint:OnDelete:SET NULL,OnUpdate:CASCADE"`
}

// BeforeCreate generates ULID before creating the branch
func (b *Branch) BeforeCreate(tx *gorm.DB) error {
	// Call BaseModel's BeforeCreate to generate ULID
	return b.BaseModel.BeforeCreate(tx)
}

// User represents a local user account (self-hosted, no external auth)
type User struct {
	BaseModel
	Email        string    `json:"email" gorm:"unique;not null"`
	PasswordHash string    `json:"-" gorm:"not null"`
	Name         string    `json:"name"`
	IsAdmin      bool      `json:"is_admin" gorm:"not null;default:false"`
	UpdatedAt    time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// AnonRule represents an anonymization rule for a database table column
// Rules are applied globally to all database restores
type AnonRule struct {
	BaseModel
	Table    string `json:"table" gorm:"not null"`
	Column   string `json:"column" gorm:"not null"`
	Template string `json:"template" gorm:"not null"` // Template with ${index} variable
}

// AutoMigrate runs database migrations for all models
func AutoMigrate(db *gorm.DB) error {
	// Collect all models
	models := []interface{}{
		&User{}, &Config{}, &Restore{}, &Branch{}, &AnonRule{},
	}

	return db.AutoMigrate(models...)
}

// FindByID safely finds a record by string ID
func FindByID[T any](db *gorm.DB, id string, model *T) error {
	return db.Where("id = ?", id).First(model).Error
}

// FindByIDWithPreload finds a record by ID with preloading
func FindByIDWithPreload[T any](db *gorm.DB, id string, model *T, preloads ...string) error {
	query := db
	for _, preload := range preloads {
		query = query.Preload(preload)
	}
	return query.Where("id = ?", id).First(model).Error
}
