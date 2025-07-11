package ldap

import (
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	ldap "github.com/go-ldap/ldap/v3"
	"go.uber.org/zap"
)

// Static errors.
var (
	ErrNotConnected = fmt.Errorf("not connected to LDAP server")
)

// Config represents LDAP client configuration.
type Config struct {
	Host          string
	Port          int
	Username      string
	Password      string
	UseSSL        bool
	SkipTLSVerify bool
	Timeout       time.Duration
	Logger        *zap.Logger
}

// Client represents an LDAP client with connection management.
type Client struct {
	config     *Config
	conn       *ldap.Conn
	mutex      sync.RWMutex
	connected  bool
	lastError  error
	logger     *zap.Logger
	retryCount int
	retryDelay time.Duration
}

// NewClient creates a new LDAP client.
func NewClient(config *Config) *Client {
	logger := config.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Client{
		config:     config,
		conn:       nil,
		mutex:      sync.RWMutex{},
		connected:  false,
		lastError:  nil,
		logger:     logger,
		retryCount: 3,
		retryDelay: 5 * time.Second,
	}
}

// Connect establishes a connection to the LDAP server.
func (c *Client) Connect() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.connected && c.conn != nil {
		return nil
	}

	var err error

	var conn *ldap.Conn

	var ldapURL string

	if c.config.UseSSL {
		ldapURL = fmt.Sprintf("ldaps://%s:%d", c.config.Host, c.config.Port)
	} else {
		ldapURL = fmt.Sprintf("ldap://%s:%d", c.config.Host, c.config.Port)
	}

	c.logger.Debug("connecting to LDAP server", zap.String("url", ldapURL))

	if c.config.UseSSL {
		tlsConfig := &tls.Config{ //nolint:exhaustruct
			InsecureSkipVerify: c.config.SkipTLSVerify, //nolint:gosec
			ServerName:         c.config.Host,
			MinVersion:         tls.VersionTLS12,
		}
		conn, err = ldap.DialURL(ldapURL, ldap.DialWithTLSConfig(tlsConfig))
	} else {
		conn, err = ldap.DialURL(ldapURL)
	}

	if err != nil {
		c.lastError = fmt.Errorf("failed to dial LDAP server: %w", err)
		c.logger.Error("failed to connect to LDAP server", zap.Error(err))

		return c.lastError
	}

	// Set connection timeout
	if c.config.Timeout > 0 {
		conn.SetTimeout(c.config.Timeout)
	}

	// Authenticate
	if err = conn.Bind(c.config.Username, c.config.Password); err != nil {
		conn.Close()

		c.lastError = fmt.Errorf("failed to authenticate: %w", err)
		c.logger.Error("failed to authenticate", zap.Error(err))

		return c.lastError
	}

	c.conn = conn
	c.connected = true
	c.lastError = nil
	c.logger.Info("successfully connected to LDAP server")

	return nil
}

// Disconnect closes the LDAP connection.
func (c *Client) Disconnect() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}

	c.connected = false
	c.logger.Debug("disconnected from LDAP server")
}

// IsConnected checks if the client is connected.
func (c *Client) IsConnected() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.connected && c.conn != nil
}

// Reconnect attempts to reconnect to the LDAP server.
func (c *Client) Reconnect() error {
	c.logger.Info("attempting to reconnect to LDAP server")
	c.Disconnect()

	return c.Connect()
}

// ConnectWithRetry attempts to connect with retry logic.
func (c *Client) ConnectWithRetry() error {
	var lastErr error

	for i := 0; i <= c.retryCount; i++ {
		if i > 0 {
			c.logger.Info("retrying connection",
				zap.Int("attempt", i),
				zap.Int("max_retries", c.retryCount))
			time.Sleep(c.retryDelay)
		}

		if err := c.Connect(); err != nil {
			lastErr = err
			c.logger.Warn("connection attempt failed",
				zap.Int("attempt", i),
				zap.Error(err))

			continue
		}

		return nil
	}

	return fmt.Errorf("failed to connect after %d retries: %w", c.retryCount, lastErr)
}

// Search performs an LDAP search.
func (c *Client) Search(searchRequest *ldap.SearchRequest) (*ldap.SearchResult, error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if !c.connected || c.conn == nil {
		return nil, ErrNotConnected
	}

	result, err := c.conn.Search(searchRequest)
	if err != nil {
		c.logger.Error("LDAP search failed", zap.Error(err))

		return nil, fmt.Errorf("LDAP search failed: %w", err)
	}

	c.logger.Debug("LDAP search completed",
		zap.Int("entries", len(result.Entries)),
		zap.String("base_dn", searchRequest.BaseDN))

	return result, nil
}

// SearchWithPaging performs an LDAP search with paging.
func (c *Client) SearchWithPaging(searchRequest *ldap.SearchRequest, pageSize uint32) (*ldap.SearchResult, error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if !c.connected || c.conn == nil {
		return nil, ErrNotConnected
	}

	// Add paging control
	pagingControl := ldap.NewControlPaging(pageSize)
	searchRequest.Controls = append(searchRequest.Controls, pagingControl)

	result, err := c.conn.Search(searchRequest)
	if err != nil {
		c.logger.Error("LDAP paged search failed", zap.Error(err))

		return nil, fmt.Errorf("LDAP paged search failed: %w", err)
	}

	c.logger.Debug("LDAP paged search completed",
		zap.Int("entries", len(result.Entries)),
		zap.Uint32("page_size", pageSize))

	return result, nil
}

// GetLastError returns the last error that occurred.
func (c *Client) GetLastError() error {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.lastError
}

// SetRetryConfig sets the retry configuration.
func (c *Client) SetRetryConfig(retryCount int, retryDelay time.Duration) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.retryCount = retryCount
	c.retryDelay = retryDelay
}

// HealthCheck performs a basic health check.
func (c *Client) HealthCheck() error {
	if !c.IsConnected() {
		return ErrNotConnected
	}

	// Perform a simple search to test connectivity
	searchRequest := ldap.NewSearchRequest(
		"", // Empty base DN for root DSE
		ldap.ScopeBaseObject,
		ldap.NeverDerefAliases,
		1, // Size limit
		0, // Time limit
		false,
		"(objectClass=*)",
		[]string{"*"},
		nil,
	)

	_, err := c.Search(searchRequest)
	if err != nil {
		c.logger.Error("health check failed", zap.Error(err))

		return fmt.Errorf("health check failed: %w", err)
	}

	return nil
}
