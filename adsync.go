package adsync

import (
	"context"
	"time"
)

// Syncer defines the main interface for Active Directory synchronization.
type Syncer interface {
	// SyncAll performs a complete synchronization of all users
	SyncAll(ctx context.Context) (<-chan User, <-chan error)

	// SyncIncremental performs incremental synchronization since the given time
	SyncIncremental(ctx context.Context, since time.Time) (<-chan User, <-chan error)

	// SyncWithCallback performs synchronization with a callback function for each user
	SyncWithCallback(ctx context.Context, callback func(User) error) error

	// Resume continues synchronization from the last saved state
	Resume(ctx context.Context) (<-chan User, <-chan error)

	// GetStats returns current synchronization statistics
	GetStats() SyncStats

	// GetResult returns the final synchronization result
	GetResult() SyncResult

	// Reset clears all saved state and starts fresh
	Reset() error

	// Close releases all resources and closes connections
	Close() error
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Host:          "",
		Port:          389,
		Username:      "",
		Password:      "",
		BaseDN:        "",
		UseSSL:        false,
		SkipTLSVerify: false,
		PageSize:      1000,
		Filter:        "(objectClass=person)",
		Attributes:    []string{"cn", "sn", "givenName", "mail", "sAMAccountName", "distinguishedName", "memberOf"},
		SessionFile:   ".adsync_session",
		StateStorage:  nil,
		Timeout:       30 * time.Second,
		RetryCount:    3,
		RetryDelay:    5 * time.Second,
		Logger:        nil,
		LogLevel:      "info",
	}
}

// NewWithDefaults creates a new Syncer with default configuration.
func NewWithDefaults(host, username, password, baseDN string) (Syncer, error) {
	config := DefaultConfig()
	config.Host = host
	config.Username = username
	config.Password = password
	config.BaseDN = baseDN

	return New(config)
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if err := c.validateRequired(); err != nil {
		return err
	}

	c.setDefaults()

	return nil
}

// validateRequired validates required fields.
func (c *Config) validateRequired() error {
	if c.Host == "" {
		return ErrInvalidConfig
	}

	if c.Username == "" {
		return ErrInvalidConfig
	}

	if c.Password == "" {
		return ErrInvalidConfig
	}

	if c.BaseDN == "" {
		return ErrInvalidConfig
	}

	if c.PageSize <= 0 {
		return ErrInvalidPageSize
	}

	return nil
}

// setDefaults sets default values for optional fields.
func (c *Config) setDefaults() {
	if c.Filter == "" {
		c.Filter = "(objectClass=person)"
	}

	if c.Timeout <= 0 {
		c.Timeout = 30 * time.Second
	}

	if c.RetryCount < 0 {
		c.RetryCount = 3
	}

	if c.RetryDelay <= 0 {
		c.RetryDelay = 5 * time.Second
	}

	if c.Port <= 0 {
		if c.UseSSL {
			c.Port = 636
		} else {
			c.Port = 389
		}
	}

	if c.SessionFile == "" {
		c.SessionFile = ".adsync_session"
	}

	if len(c.Attributes) == 0 {
		c.Attributes = []string{"cn", "sn", "givenName", "mail", "sAMAccountName", "distinguishedName", "memberOf"}
	}
}
