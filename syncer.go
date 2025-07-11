package adsync

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/romodevx/go-adsync/internal/ldap"
	"github.com/romodevx/go-adsync/internal/state"
	"go.uber.org/zap"
)

// syncer implements the Syncer interface.
type syncer struct {
	config    *Config
	client    *Client
	logger    *zap.Logger
	storage   StateStorage
	stats     SyncStats
	result    SyncResult
	mutex     sync.RWMutex
	isRunning bool
	startTime time.Time
}

// New creates a new Syncer instance.
func New(config *Config) (Syncer, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	var logger *zap.Logger
	if config.Logger != nil {
		logger = config.Logger
	} else {
		var err error

		logger, err = createDefaultLogger(config.LogLevel)
		if err != nil {
			return nil, fmt.Errorf("failed to create logger: %w", err)
		}
	}

	var storage StateStorage
	if config.StateStorage != nil {
		storage = config.StateStorage
	} else {
		// Default to memory (safer and simpler)
		storage = state.NewMemoryStateStorage()
	}

	// Create LDAP client
	client, err := NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create LDAP client: %w", err)
	}

	return &syncer{
		config:  config,
		client:  client,
		logger:  logger,
		storage: storage,
		stats: SyncStats{
			StartTime:      time.Time{},
			EndTime:        time.Time{},
			Duration:       0,
			TotalPages:     0,
			CurrentPage:    0,
			UsersPerSecond: 0,
			IsComplete:     false,
		},
		result: SyncResult{
			TotalUsers:     0,
			ProcessedUsers: 0,
			SkippedUsers:   0,
			ErrorCount:     0,
			Duration:       0,
			LastCookie:     "",
			Errors:         make([]error, 0),
		},
		mutex:     sync.RWMutex{},
		isRunning: false,
		startTime: time.Time{},
	}, nil
}

// initializeSync sets up the sync operation.
func (s *syncer) initializeSync() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.isRunning = true
	s.startTime = time.Now()
	s.stats.StartTime = s.startTime
}

// finalizeSync completes the sync operation.
func (s *syncer) finalizeSync() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.isRunning = false
	s.stats.EndTime = time.Now()
	s.stats.Duration = s.stats.EndTime.Sub(s.stats.StartTime)
	s.stats.IsComplete = true
}

// SyncAll performs a complete synchronization of all users.
func (s *syncer) SyncAll(ctx context.Context) (<-chan User, <-chan error) {
	userChan := make(chan User, 100)
	errChan := make(chan error, 10)

	go func() {
		defer close(userChan)
		defer close(errChan)

		s.initializeSync()
		defer s.finalizeSync()

		// Use the LDAP client to sync users
		ldapUserChan, ldapErrChan := s.client.SyncAll(ctx)

		for {
			select {
			case ldapUser, ok := <-ldapUserChan:
				if !ok {
					ldapUserChan = nil

					continue
				}

				// Convert LDAP user to public User type
				user := s.convertLDAPUser(ldapUser)

				// Update stats
				s.mutex.Lock()
				s.result.ProcessedUsers++
				s.mutex.Unlock()

				userChan <- user

			case err, ok := <-ldapErrChan:
				if !ok {
					ldapErrChan = nil

					continue
				}

				if err != nil {
					s.mutex.Lock()
					s.result.ErrorCount++
					s.result.Errors = append(s.result.Errors, err)
					s.mutex.Unlock()
					errChan <- err
				}

			case <-ctx.Done():
				return
			}

			if ldapUserChan == nil && ldapErrChan == nil {
				break
			}
		}
	}()

	return userChan, errChan
}

// SyncIncremental performs incremental synchronization since the given time.
func (s *syncer) SyncIncremental(ctx context.Context, since time.Time) (<-chan User, <-chan error) {
	userChan := make(chan User, 100)
	errChan := make(chan error, 10)

	go func() {
		defer close(userChan)
		defer close(errChan)

		// Use the LDAP client to sync users incrementally
		ldapUserChan, ldapErrChan := s.client.SyncIncremental(ctx, since)

		for {
			select {
			case ldapUser, ok := <-ldapUserChan:
				if !ok {
					ldapUserChan = nil

					continue
				}

				// Convert LDAP user to public User type
				user := s.convertLDAPUser(ldapUser)

				// Update stats
				s.mutex.Lock()
				s.result.ProcessedUsers++
				s.mutex.Unlock()

				userChan <- user

			case err, ok := <-ldapErrChan:
				if !ok {
					ldapErrChan = nil

					continue
				}

				if err != nil {
					s.mutex.Lock()
					s.result.ErrorCount++
					s.result.Errors = append(s.result.Errors, err)
					s.mutex.Unlock()
					errChan <- err
				}

			case <-ctx.Done():
				return
			}

			if ldapUserChan == nil && ldapErrChan == nil {
				break
			}
		}
	}()

	return userChan, errChan
}

// SyncWithCallback performs synchronization with a callback function for each user.
func (s *syncer) SyncWithCallback(ctx context.Context, callback func(User) error) error {
	userChan, errChan := s.SyncAll(ctx)

	return s.processCallbackChannels(ctx, userChan, errChan, callback)
}

// processCallbackChannels processes user and error channels with callback.
func (s *syncer) processCallbackChannels(
	ctx context.Context,
	userChan <-chan User,
	errChan <-chan error,
	callback func(User) error,
) error {
	for {
		select {
		case user, ok := <-userChan:
			if err := s.handleUserCallback(user, ok, callback, &userChan); err != nil {
				return err
			}
		case err, ok := <-errChan:
			if err := s.handleErrorCallback(err, ok, &errChan); err != nil {
				return err
			}
		case <-ctx.Done():
			return fmt.Errorf("context canceled: %w", ctx.Err())
		}

		if userChan == nil && errChan == nil {
			break
		}
	}

	return nil
}

// handleUserCallback handles user callback processing.
func (s *syncer) handleUserCallback(user User, ok bool, callback func(User) error, userChan *<-chan User) error {
	if !ok {
		*userChan = nil

		return nil
	}

	if err := callback(user); err != nil {
		return fmt.Errorf("callback error: %w", err)
	}

	return nil
}

// handleErrorCallback handles error callback processing.
func (s *syncer) handleErrorCallback(err error, ok bool, errChan *<-chan error) error {
	if !ok {
		*errChan = nil

		return nil
	}

	if err != nil {
		return err
	}

	return nil
}

// Resume continues synchronization from the last saved state (memory only).
func (s *syncer) Resume(ctx context.Context) (<-chan User, <-chan error) {
	userChan := make(chan User, 100)
	errChan := make(chan error, 10)

	go func() {
		defer close(userChan)
		defer close(errChan)

		if err := s.initializeResumeState(ctx, errChan); err != nil {
			return
		}

		ldapUserChan, ldapErrChan := s.client.SyncAll(ctx)
		s.processResumeChannels(ctx, userChan, errChan, ldapUserChan, ldapErrChan)
	}()

	return userChan, errChan
}

// initializeResumeState initializes the resume state from storage.
func (s *syncer) initializeResumeState(ctx context.Context, errChan chan<- error) error {
	var lastCookie []byte

	var pageIndex int

	_ = s.storage.Load("last_cookie", &lastCookie)
	_ = s.storage.Load("page_index", &pageIndex)

	s.client.SetLastCookie(lastCookie)
	s.client.SetPageIndex(0)

	// Try to skip to the correct page if lastCookie is invalid
	if err := s.client.SkipToPage(ctx, pageIndex); err != nil {
		errChan <- fmt.Errorf("failed to skip to page %d: %w", pageIndex, err)

		return err
	}

	return nil
}

// processResumeChannels processes the resume channels.
func (s *syncer) processResumeChannels(
	ctx context.Context,
	userChan chan<- User,
	errChan chan<- error,
	ldapUserChan <-chan *ldap.User,
	ldapErrChan <-chan error,
) {
	var pageIndex int
	_ = s.storage.Load("page_index", &pageIndex)

	pageCounter := pageIndex

	for {
		select {
		case ldapUser, ok := <-ldapUserChan:
			ldapUserChan = s.handleResumeUser(ldapUser, ok, userChan, &pageCounter, ldapUserChan)
		case err, ok := <-ldapErrChan:
			ldapErrChan = s.handleResumeError(err, ok, errChan, ldapErrChan)
		case <-ctx.Done():
			return
		}

		if ldapUserChan == nil && ldapErrChan == nil {
			break
		}
	}
}

// handleResumeUser handles a user during resume processing.
func (s *syncer) handleResumeUser(
	ldapUser *ldap.User,
	ok bool,
	userChan chan<- User,
	pageCounter *int,
	ldapUserChan <-chan *ldap.User,
) <-chan *ldap.User {
	if !ok {
		return nil
	}

	user := s.convertLDAPUser(ldapUser)
	userChan <- user

	s.mutex.Lock()
	s.result.ProcessedUsers++
	s.mutex.Unlock()

	// Save state after each page
	if s.result.ProcessedUsers%s.config.PageSize == 0 {
		*pageCounter++
		_ = s.storage.Save("last_cookie", s.client.GetLastCookie())
		_ = s.storage.Save("page_index", *pageCounter)
	}

	return ldapUserChan
}

// handleResumeError handles an error during resume processing.
func (s *syncer) handleResumeError(
	err error,
	ok bool,
	errChan chan<- error,
	ldapErrChan <-chan error,
) <-chan error {
	if !ok {
		return nil
	}

	if err != nil {
		errChan <- err
	}

	return ldapErrChan
}

// Reset clears all saved state and starts fresh (memory only).
func (s *syncer) Reset() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	_ = s.storage.Delete("last_cookie")
	_ = s.storage.Delete("page_index")
	// Reset internal state
	s.stats = SyncStats{
		StartTime:      time.Time{},
		EndTime:        time.Time{},
		Duration:       0,
		TotalPages:     0,
		CurrentPage:    0,
		UsersPerSecond: 0,
		IsComplete:     false,
	}
	s.result = SyncResult{
		TotalUsers:     0,
		ProcessedUsers: 0,
		SkippedUsers:   0,
		ErrorCount:     0,
		Duration:       0,
		LastCookie:     "",
		Errors:         make([]error, 0),
	}
	s.isRunning = false

	return nil
}

// Close releases all resources and closes connections.
func (s *syncer) Close() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.isRunning {
		s.isRunning = false
	}

	// Close LDAP client
	if s.client != nil {
		if err := s.client.Close(); err != nil {
			s.logger.Warn("failed to close LDAP client", zap.Error(err))
		}
	}

	return nil
}

// convertLDAPUser converts an LDAP user to the public User type.
func (s *syncer) convertLDAPUser(ldapUser *ldap.User) User {
	return User{
		DN:           ldapUser.DN,
		Username:     ldapUser.Username,
		Email:        ldapUser.Email,
		FirstName:    ldapUser.FirstName,
		LastName:     ldapUser.LastName,
		DisplayName:  ldapUser.DisplayName,
		Groups:       ldapUser.Groups,
		Attributes:   ldapUser.Attributes,
		LastModified: ldapUser.LastModified,
	}
}

// createDefaultLogger creates a default logger with the specified level.
func createDefaultLogger(level string) (*zap.Logger, error) {
	config := zap.NewProductionConfig()

	switch level {
	case "debug":
		config.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		config.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		config.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	logger, err := config.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build logger: %w", err)
	}

	return logger, nil
}

// GetResult returns the final synchronization result.
func (s *syncer) GetResult() SyncResult {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return s.result
}

// GetStats returns current synchronization statistics.
func (s *syncer) GetStats() SyncStats {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return s.stats
}
