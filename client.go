package adsync

import (
	"context"
	"fmt"
	"time"

	ldapv3 "github.com/go-ldap/ldap/v3"
	"github.com/romodevx/go-adsync/internal/ldap"
	"go.uber.org/zap"
)

// Client represents the main LDAP client for the library.
type Client struct {
	ldapClient *ldap.Client
	paginator  *ldap.Paginator
	parser     *ldap.UserParser
	config     *Config
	logger     *zap.Logger
}

// NewClient creates a new LDAP client.
func NewClient(config *Config) (*Client, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Create logger
	logger := config.Logger
	if logger == nil {
		var err error

		logger, err = createDefaultLogger(config.LogLevel)
		if err != nil {
			return nil, fmt.Errorf("failed to create logger: %w", err)
		}
	}

	// Create LDAP client config
	ldapConfig := &ldap.Config{
		Host:          config.Host,
		Port:          config.Port,
		Username:      config.Username,
		Password:      config.Password,
		UseSSL:        config.UseSSL,
		SkipTLSVerify: config.SkipTLSVerify,
		Timeout:       config.Timeout,
		Logger:        logger,
	}

	// Create LDAP client
	ldapClient := ldap.NewClient(ldapConfig)

	// Create user parser
	parser := ldap.NewUserParser(logger)

	// Create paginator config
	paginatorConfig := &ldap.PaginatorConfig{
		BaseDN:     config.BaseDN,
		Filter:     config.Filter,
		Attributes: config.Attributes,
		PageSize:   uint32(config.PageSize), //nolint:gosec
		Logger:     logger,
	}

	// Create paginator
	paginator := ldap.NewPaginator(ldapClient, paginatorConfig)

	client := &Client{
		ldapClient: ldapClient,
		paginator:  paginator,
		parser:     parser,
		config:     config,
		logger:     logger,
	}

	return client, nil
}

// Connect establishes connection to LDAP server.
func (c *Client) Connect() error {
	if err := c.ldapClient.ConnectWithRetry(); err != nil {
		return fmt.Errorf("failed to connect to LDAP: %w", err)
	}

	return nil
}

// Disconnect closes the LDAP connection.
func (c *Client) Disconnect() {
	c.ldapClient.Disconnect()
}

// IsConnected checks if client is connected.
func (c *Client) IsConnected() bool {
	return c.ldapClient.IsConnected()
}

// SyncAll synchronizes all users with pagination.
func (c *Client) SyncAll(ctx context.Context) (<-chan *ldap.User, <-chan error) {
	userChan := make(chan *ldap.User, 100)
	errChan := make(chan error, 10)

	go func() {
		defer close(userChan)
		defer close(errChan)

		if err := c.performSync(ctx, userChan, false, time.Time{}); err != nil {
			errChan <- err
		}
	}()

	return userChan, errChan
}

// performSync handles the actual synchronization logic.
func (c *Client) performSync(ctx context.Context, userChan chan<- *ldap.User, incremental bool, since time.Time) error {
	c.logger.Info("starting synchronization")

	if err := c.ensureConnection(); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	if !incremental {
		c.paginator.Reset()
	}

	return c.processPagination(ctx, userChan, incremental, since)
}

// ensureConnection ensures LDAP connection is established.
func (c *Client) ensureConnection() error {
	if !c.IsConnected() {
		return c.Connect()
	}

	return nil
}

// processPagination handles the pagination loop.
func (c *Client) processPagination(ctx context.Context, userChan chan<- *ldap.User, _ bool, _ time.Time) error {
	pageCount := 0
	totalUsers := 0

	for c.paginator.HasMore() {
		if err := c.checkContext(ctx); err != nil {
			return err
		}

		result, err := c.paginator.NextPage()
		if err != nil {
			return fmt.Errorf("failed to get page %d: %w", pageCount+1, err)
		}

		pageCount++
		if err := c.processPage(ctx, result, userChan, pageCount, &totalUsers); err != nil {
			return err
		}
	}

	c.logger.Info("synchronization completed",
		zap.Int("total_pages", pageCount),
		zap.Int("total_users", totalUsers))

	return nil
}

// processPage processes a single page of results.
func (c *Client) processPage(
	ctx context.Context,
	result *ldapv3.SearchResult,
	userChan chan<- *ldap.User,
	pageCount int,
	totalUsers *int,
) error {
	c.logger.Debug("processing page", zap.Int("page", pageCount), zap.Int("entries", len(result.Entries)))

	users, err := c.parser.ParseEntries(result.Entries)
	if err != nil {
		c.logger.Warn("some entries failed to parse", zap.Error(err))
	}

	return c.sendUsers(ctx, users, userChan, totalUsers, pageCount)
}

// sendUsers sends parsed users to the channel.
func (c *Client) sendUsers(
	ctx context.Context,
	users []*ldap.User,
	userChan chan<- *ldap.User,
	totalUsers *int,
	pageCount int,
) error {
	for _, user := range users {
		if err := c.checkContext(ctx); err != nil {
			return err
		}

		userChan <- user

		*totalUsers++
	}

	c.logger.Debug("page processed",
		zap.Int("page", pageCount),
		zap.Int("users_in_page", len(users)),
		zap.Int("total_users", *totalUsers))

	return nil
}

// checkContext checks if context is canceled.
func (c *Client) checkContext(ctx context.Context) error {
	select {
	case <-ctx.Done():
		c.logger.Info("synchronization canceled")

		return ctx.Err()
	default:
		return nil
	}
}

// SyncIncremental synchronizes users modified since a specific time.
func (c *Client) SyncIncremental(ctx context.Context, since time.Time) (<-chan *ldap.User, <-chan error) {
	userChan := make(chan *ldap.User, 100)
	errChan := make(chan error, 10)

	go func() {
		defer close(userChan)
		defer close(errChan)

		if err := c.performIncrementalSync(ctx, userChan, since); err != nil {
			errChan <- err
		}
	}()

	return userChan, errChan
}

// performIncrementalSync handles incremental synchronization.
func (c *Client) performIncrementalSync(ctx context.Context, userChan chan<- *ldap.User, since time.Time) error {
	c.logger.Info("starting incremental synchronization", zap.Time("since", since))

	if err := c.ensureConnection(); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	incrementalPaginator := c.createIncrementalPaginator(since)

	return c.processIncrementalPagination(ctx, userChan, incrementalPaginator)
}

// createIncrementalPaginator creates paginator for incremental sync.
func (c *Client) createIncrementalPaginator(since time.Time) *ldap.Paginator {
	modifiedFilter := fmt.Sprintf("(&%s(whenChanged>=%s))",
		c.config.Filter,
		since.Format("20060102150405.0Z"))

	paginatorConfig := &ldap.PaginatorConfig{
		BaseDN:     c.config.BaseDN,
		Filter:     modifiedFilter,
		Attributes: c.config.Attributes,
		PageSize:   uint32(c.config.PageSize), //nolint:gosec
		Logger:     c.logger,
	}

	return ldap.NewPaginator(c.ldapClient, paginatorConfig)
}

// processIncrementalPagination processes incremental pagination.
func (c *Client) processIncrementalPagination(
	ctx context.Context,
	userChan chan<- *ldap.User,
	paginator *ldap.Paginator,
) error {
	pageCount := 0
	totalUsers := 0

	for paginator.HasMore() {
		if err := c.checkContext(ctx); err != nil {
			return err
		}

		result, err := paginator.NextPage()
		if err != nil {
			return fmt.Errorf("failed to get incremental page %d: %w", pageCount+1, err)
		}

		pageCount++
		if err := c.processIncrementalPage(ctx, result, userChan, pageCount, &totalUsers); err != nil {
			return err
		}
	}

	c.logger.Info("incremental synchronization completed",
		zap.Int("total_pages", pageCount),
		zap.Int("total_users", totalUsers))

	return nil
}

// processIncrementalPage processes a single incremental page.
func (c *Client) processIncrementalPage(
	ctx context.Context,
	result *ldapv3.SearchResult,
	userChan chan<- *ldap.User,
	pageCount int,
	totalUsers *int,
) error {
	c.logger.Debug("processing incremental page", zap.Int("page", pageCount), zap.Int("entries", len(result.Entries)))

	users, err := c.parser.ParseEntries(result.Entries)
	if err != nil {
		c.logger.Warn("some entries failed to parse", zap.Error(err))
	}

	return c.sendUsers(ctx, users, userChan, totalUsers, pageCount)
}

// GetLastCookie returns the last pagination cookie.
func (c *Client) GetLastCookie() []byte {
	return c.paginator.GetLastCookie()
}

// SetLastCookie sets the last pagination cookie for resuming.
func (c *Client) SetLastCookie(cookie []byte) {
	c.paginator.SetLastCookie(cookie)
}

// GetPageCount returns the number of pages processed.
func (c *Client) GetPageCount() int {
	return c.paginator.GetPageCount()
}

// HealthCheck performs a health check on the LDAP connection.
func (c *Client) HealthCheck() error {
	if err := c.ldapClient.HealthCheck(); err != nil {
		return fmt.Errorf("LDAP health check failed: %w", err)
	}

	return nil
}

// Close closes the client and releases resources.
func (c *Client) Close() error {
	c.Disconnect()

	return nil
}
