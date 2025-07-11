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

// syncer implements the Syncer interface
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

// New creates a new Syncer instance
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
		storage = state.NewFileStateStorage(config.SessionFile)
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

// initializeSync sets up the sync operation
func (s *syncer) initializeSync() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.isRunning = true
	s.startTime = time.Now()
	s.stats.StartTime = s.startTime
}

// finalizeSync completes the sync operation
func (s *syncer) finalizeSync() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.isRunning = false
	s.stats.EndTime = time.Now()
	s.stats.Duration = s.stats.EndTime.Sub(s.stats.StartTime)
	s.stats.IsComplete = true
}

// SyncAll performs a complete synchronization of all users
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

// SyncIncremental performs incremental synchronization since the given time
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

// SyncWithCallback performs synchronization with a callback function for each user
func (s *syncer) SyncWithCallback(ctx context.Context, callback func(User) error) error {
	userChan, errChan := s.SyncAll(ctx)
	return s.processCallbackChannels(ctx, userChan, errChan, callback)
}

// processCallbackChannels processes user and error channels with callback
func (s *syncer) processCallbackChannels(
	ctx context.Context,
	userChan <-chan User,
	errChan <-chan error,
	callback func(User) error,
) error {
	for {
		select {
		case user, ok := <-userChan:
			if !ok {
				userChan = nil

				continue
			}
			if err := callback(user); err != nil {
				return fmt.Errorf("callback error: %w", err)
			}
		case err, ok := <-errChan:
			if !ok {
				errChan = nil

				continue
			}
			if err != nil {
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

// Resume continues synchronization from the last saved state
func (s *syncer) Resume(ctx context.Context) (<-chan User, <-chan error) {
	userChan := make(chan User, 100)
	errChan := make(chan error, 10)

	go func() {
		defer close(userChan)
		defer close(errChan)

		s.loadLastCookieState()
		ldapUserChan, ldapErrChan := s.client.SyncAll(ctx)
		s.processResumeChannels(ctx, userChan, errChan, ldapUserChan, ldapErrChan)
		s.saveProgressWithLog("failed to save final progress")
	}()

	return userChan, errChan
}

// loadLastCookieState loads the last cookie from storage if available
func (s *syncer) loadLastCookieState() {
	var lastCookie []byte
	if err := s.storage.Load("last_cookie", &lastCookie); err == nil {
		s.client.SetLastCookie(lastCookie)
		s.logger.Info("resumed from saved state")
	} else {
		s.logger.Info("no saved state found, starting fresh")
	}
}

// processResumeChannels processes the channels from LDAP client
func (s *syncer) processResumeChannels(
	ctx context.Context,
	userChan chan<- User,
	errChan chan<- error,
	ldapUserChan <-chan *ldap.User,
	ldapErrChan <-chan error,
) {
	for {
		select {
		case ldapUser, ok := <-ldapUserChan:
			if !ok {
				ldapUserChan = nil

				continue
			}

			s.processResumeUser(ldapUser, userChan)

		case err, ok := <-ldapErrChan:
			if !ok {
				ldapErrChan = nil

				continue
			}

			s.processResumeError(err, errChan)

		case <-ctx.Done():
			s.saveProgressWithLog("failed to save progress on exit")
			return
		}

		if ldapUserChan == nil && ldapErrChan == nil {
			break
		}
	}
}

// processResumeUser processes a single user during resume
func (s *syncer) processResumeUser(ldapUser *ldap.User, userChan chan<- User) {
	user := s.convertLDAPUser(ldapUser)

	s.mutex.Lock()
	s.result.ProcessedUsers++
	shouldSave := s.result.ProcessedUsers%100 == 0
	s.mutex.Unlock()

	userChan <- user

	if shouldSave {
		s.saveProgressWithLog("failed to save progress")
	}
}

// processResumeError processes an error during resume
func (s *syncer) processResumeError(err error, errChan chan<- error) {
	if err != nil {
		s.mutex.Lock()
		s.result.ErrorCount++
		s.result.Errors = append(s.result.Errors, err)
		s.mutex.Unlock()
		errChan <- err
	}
}

// saveProgressWithLog saves progress and logs any errors
func (s *syncer) saveProgressWithLog(logMessage string) {
	if err := s.saveProgress(); err != nil {
		s.logger.Warn(logMessage, zap.Error(err))
	}
}

// GetStats returns current synchronization statistics
func (s *syncer) GetStats() SyncStats {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	stats := s.stats
	if s.isRunning {
		stats.Duration = time.Since(s.startTime)
		if stats.Duration.Seconds() > 0 {
			stats.UsersPerSecond = float64(stats.TotalPages*s.config.PageSize) / stats.Duration.Seconds()
		}
	}
	return stats
}

// GetResult returns the final synchronization result
func (s *syncer) GetResult() SyncResult {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return s.result
}

// Reset clears all saved state and starts fresh
func (s *syncer) Reset() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Clear all state keys
	keys := []string{"last_cookie", "processed_users", "current_page", "total_pages", "sync_stats"}
	for _, key := range keys {
		if err := s.storage.Delete(key); err != nil {
			s.logger.Warn("failed to delete state key", zap.String("key", key), zap.Error(err))
		}
	}

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

// Close releases all resources and closes connections
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

// convertLDAPUser converts an LDAP user to the public User type
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

// saveProgress saves the current synchronization progress
func (s *syncer) saveProgress() error {
	cookie := s.client.GetLastCookie()
	if cookie != nil {
		if err := s.storage.Save("last_cookie", cookie); err != nil {
			return fmt.Errorf("failed to save last cookie: %w", err)
		}
	}

	if err := s.storage.Save("sync_stats", s.stats); err != nil {
		return fmt.Errorf("failed to save sync stats: %w", err)
	}

	if err := s.storage.Save("sync_result", s.result); err != nil {
		return fmt.Errorf("failed to save sync result: %w", err)
	}

	return nil
}

// createDefaultLogger creates a default logger with the specified level
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
