package adsync

import (
	"time"

	"go.uber.org/zap"
)

// Config represents the configuration for the AD synchronizer
type Config struct {
	// LDAP Connection settings
	Host          string `json:"host"`
	Port          int    `json:"port,omitempty"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	BaseDN        string `json:"base_dn"`
	UseSSL        bool   `json:"use_ssl,omitempty"`
	SkipTLSVerify bool   `json:"skip_tls_verify,omitempty"`

	// Pagination and filtering
	PageSize   int      `json:"page_size,omitempty"`
	Filter     string   `json:"filter,omitempty"`
	Attributes []string `json:"attributes,omitempty"`

	// Session management
	SessionFile  string       `json:"session_file,omitempty"`
	StateStorage StateStorage `json:"-"`

	// Timeouts and retries
	Timeout    time.Duration `json:"timeout,omitempty"`
	RetryCount int           `json:"retry_count,omitempty"`
	RetryDelay time.Duration `json:"retry_delay,omitempty"`

	// Logging
	Logger   *zap.Logger `json:"-"`
	LogLevel string      `json:"log_level,omitempty"`
}

// User represents an Active Directory user
type User struct {
	DN           string            `json:"dn"`
	Username     string            `json:"username"`
	Email        string            `json:"email"`
	FirstName    string            `json:"first_name"`
	LastName     string            `json:"last_name"`
	DisplayName  string            `json:"display_name"`
	Groups       []string          `json:"groups"`
	Attributes   map[string]string `json:"attributes"`
	LastModified time.Time         `json:"last_modified"`
}

// SyncResult represents the result of a synchronization operation
type SyncResult struct {
	TotalUsers     int           `json:"total_users"`
	ProcessedUsers int           `json:"processed_users"`
	SkippedUsers   int           `json:"skipped_users"`
	ErrorCount     int           `json:"error_count"`
	Duration       time.Duration `json:"duration"`
	LastCookie     string        `json:"last_cookie"`
	Errors         []error       `json:"errors"`
}

// StateStorage defines the interface for state persistence
type StateStorage interface {
	Save(key string, value any) error
	Load(key string, dest any) error
	Delete(key string) error
	Exists(key string) bool
}

// SyncMode defines the type of synchronization to perform
type SyncMode int

const (
	SyncModeComplete SyncMode = iota
	SyncModeIncremental
	SyncModeResume
)

// SyncStats represents synchronization statistics
type SyncStats struct {
	StartTime      time.Time     `json:"start_time"`
	EndTime        time.Time     `json:"end_time"`
	Duration       time.Duration `json:"duration"`
	TotalPages     int           `json:"total_pages"`
	CurrentPage    int           `json:"current_page"`
	UsersPerSecond float64       `json:"users_per_second"`
	IsComplete     bool          `json:"is_complete"`
}
