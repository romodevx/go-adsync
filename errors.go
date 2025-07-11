package adsync

import (
	"errors"
	"fmt"
)

// Standard errors.
var (
	ErrInvalidConfig        = errors.New("invalid configuration")
	ErrConnectionFailed     = errors.New("ldap connection failed")
	ErrAuthFailed           = errors.New("ldap authentication failed")
	ErrSearchFailed         = errors.New("ldap search failed")
	ErrSessionNotFound      = errors.New("session not found")
	ErrStateCorrupted       = errors.New("session state corrupted")
	ErrInvalidUser          = errors.New("invalid user data")
	ErrStorageError         = errors.New("storage operation failed")
	ErrTimeout              = errors.New("operation timeout")
	ErrCanceled             = errors.New("operation canceled")
	ErrNotConnected         = errors.New("not connected to ldap server")
	ErrInvalidPageSize      = errors.New("invalid page size")
	ErrInvalidFilter        = errors.New("invalid ldap filter")
	ErrTLSError             = errors.New("tls connection error")
	ErrMaxRetriesReached    = errors.New("maximum retries reached")
	ErrNoMorePages          = errors.New("no more pages available")
	ErrEntryNil             = errors.New("entry is nil")
	ErrUsernameRequired     = errors.New("username is required")
	ErrDNRequired           = errors.New("DN is required")
	ErrTimestampParseFailed = errors.New("unable to parse timestamp")
)

// ConnectionError represents LDAP connection errors.
type ConnectionError struct {
	Host   string
	Port   int
	Reason string
	Err    error
}

func (e *ConnectionError) Error() string {
	return fmt.Sprintf("connection error to %s:%d: %s", e.Host, e.Port, e.Reason)
}

func (e *ConnectionError) Unwrap() error {
	return e.Err
}

// AuthError represents authentication errors.
type AuthError struct {
	Username string
	Reason   string
	Err      error
}

func (e *AuthError) Error() string {
	return fmt.Sprintf("authentication failed for user %s: %s", e.Username, e.Reason)
}

func (e *AuthError) Unwrap() error {
	return e.Err
}

// SearchError represents LDAP search errors.
type SearchError struct {
	Filter string
	BaseDN string
	Reason string
	Err    error
}

func (e *SearchError) Error() string {
	return fmt.Sprintf("search failed: filter=%s, basedn=%s, reason=%s", e.Filter, e.BaseDN, e.Reason)
}

func (e *SearchError) Unwrap() error {
	return e.Err
}

// PaginationError represents pagination-related errors.
type PaginationError struct {
	Page   int
	Cookie string
	Reason string
	Err    error
}

func (e *PaginationError) Error() string {
	return fmt.Sprintf("pagination error on page %d: %s", e.Page, e.Reason)
}

func (e *PaginationError) Unwrap() error {
	return e.Err
}

// StateError represents state management errors.
type StateError struct {
	Operation string
	Key       string
	Reason    string
	Err       error
}

func (e *StateError) Error() string {
	return fmt.Sprintf("state %s error for key %s: %s", e.Operation, e.Key, e.Reason)
}

func (e *StateError) Unwrap() error {
	return e.Err
}

// RetryableError indicates that an operation can be retried.
type RetryableError struct {
	Err       error
	Retryable bool
}

func (e *RetryableError) Error() string {
	return e.Err.Error()
}

func (e *RetryableError) Unwrap() error {
	return e.Err
}

func (e *RetryableError) IsRetryable() bool {
	return e.Retryable
}

// NewConnectionError creates a new connection error.
func NewConnectionError(host string, port int, reason string, err error) *ConnectionError {
	return &ConnectionError{
		Host:   host,
		Port:   port,
		Reason: reason,
		Err:    err,
	}
}

// NewAuthError creates a new authentication error.
func NewAuthError(username, reason string, err error) *AuthError {
	return &AuthError{
		Username: username,
		Reason:   reason,
		Err:      err,
	}
}

// NewSearchError creates a new search error.
func NewSearchError(filter, baseDN, reason string, err error) *SearchError {
	return &SearchError{
		Filter: filter,
		BaseDN: baseDN,
		Reason: reason,
		Err:    err,
	}
}

// NewPaginationError creates a new pagination error.
func NewPaginationError(page int, cookie, reason string, err error) *PaginationError {
	return &PaginationError{
		Page:   page,
		Cookie: cookie,
		Reason: reason,
		Err:    err,
	}
}

// NewStateError creates a new state error.
func NewStateError(operation, key, reason string, err error) *StateError {
	return &StateError{
		Operation: operation,
		Key:       key,
		Reason:    reason,
		Err:       err,
	}
}

// NewRetryableError creates a new retryable error.
func NewRetryableError(err error, retryable bool) *RetryableError {
	return &RetryableError{
		Err:       err,
		Retryable: retryable,
	}
}

// IsRetryable checks if an error is retryable.
func IsRetryable(err error) bool {
	var retryableErr *RetryableError
	if errors.As(err, &retryableErr) {
		return retryableErr.IsRetryable()
	}

	return false
}
