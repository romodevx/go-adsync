package ldap

import (
	"fmt"
	"sync"

	ldap "github.com/go-ldap/ldap/v3"
	"go.uber.org/zap"
)

// Static errors.
var (
	ErrNoMorePages = fmt.Errorf("no more pages available")
)

// PaginatorConfig represents configuration for the paginator.
type PaginatorConfig struct {
	BaseDN     string
	Filter     string
	Attributes []string
	PageSize   uint32
	Logger     *zap.Logger
}

// Paginator handles LDAP pagination with lastCookie support.
type Paginator struct {
	client     *Client
	config     *PaginatorConfig
	lastCookie []byte
	hasMore    bool
	mutex      sync.RWMutex
	logger     *zap.Logger
	pageCount  int
}

// NewPaginator creates a new LDAP paginator.
func NewPaginator(client *Client, config *PaginatorConfig) *Paginator {
	logger := config.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Paginator{
		client:     client,
		config:     config,
		lastCookie: nil,
		hasMore:    true,
		mutex:      sync.RWMutex{},
		logger:     logger,
		pageCount:  0,
	}
}

// NextPage retrieves the next page of results.
func (p *Paginator) NextPage() (*ldap.SearchResult, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if !p.hasMore {
		return nil, ErrNoMorePages
	}

	// Create search request
	searchRequest := ldap.NewSearchRequest(
		p.config.BaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0, // No size limit
		0, // No time limit
		false,
		p.config.Filter,
		p.config.Attributes,
		nil,
	)

	// Add paging control with lastCookie
	pagingControl := ldap.NewControlPaging(p.config.PageSize)
	if p.lastCookie != nil {
		pagingControl.SetCookie(p.lastCookie)
	}

	searchRequest.Controls = append(searchRequest.Controls, pagingControl)

	p.logger.Debug("fetching next page",
		zap.Int("page_number", p.pageCount+1),
		zap.Uint32("page_size", p.config.PageSize),
		zap.Bool("has_cookie", p.lastCookie != nil))

	// Perform the search
	result, err := p.client.Search(searchRequest)
	if err != nil {
		p.logger.Error("failed to fetch page", zap.Error(err))
		return nil, fmt.Errorf("failed to fetch page: %w", err)
	}

	// Process paging control from response
	p.lastCookie = nil
	p.hasMore = false

	for _, control := range result.Controls {
		if pagingResult, ok := control.(*ldap.ControlPaging); ok {
			p.lastCookie = pagingResult.Cookie
			p.hasMore = len(p.lastCookie) > 0

			break
		}
	}

	p.pageCount++

	p.logger.Debug("page fetched successfully",
		zap.Int("page_number", p.pageCount),
		zap.Int("entries", len(result.Entries)),
		zap.Bool("has_more", p.hasMore))

	return result, nil
}

// HasMore returns true if there are more pages available.
func (p *Paginator) HasMore() bool {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.hasMore
}

// GetLastCookie returns the last cookie received
func (p *Paginator) GetLastCookie() []byte {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	if p.lastCookie == nil {
		return nil
	}
	// Return a copy to prevent modification
	cookie := make([]byte, len(p.lastCookie))
	copy(cookie, p.lastCookie)
	return cookie
}

// SetLastCookie sets the last cookie for resuming pagination
func (p *Paginator) SetLastCookie(cookie []byte) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if cookie == nil {
		p.lastCookie = nil
		p.hasMore = true
		return
	}

	p.lastCookie = make([]byte, len(cookie))
	copy(p.lastCookie, cookie)
	p.hasMore = true

	p.logger.Debug("last cookie set for resuming pagination",
		zap.Bool("has_cookie", len(p.lastCookie) > 0))
}

// GetPageCount returns the number of pages fetched
func (p *Paginator) GetPageCount() int {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.pageCount
}

// Reset resets the paginator to start from the beginning
func (p *Paginator) Reset() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.lastCookie = nil
	p.hasMore = true
	p.pageCount = 0

	p.logger.Debug("paginator reset")
}

// AllPages returns all pages as a channel
func (p *Paginator) AllPages() (<-chan *ldap.SearchResult, <-chan error) {
	resultChan := make(chan *ldap.SearchResult, 10)
	errChan := make(chan error, 1)

	go func() {
		defer close(resultChan)
		defer close(errChan)

		for p.HasMore() {
			result, err := p.NextPage()
			if err != nil {
				errChan <- err
				return
			}

			if len(result.Entries) > 0 {
				resultChan <- result
			}
		}
	}()

	return resultChan, errChan
}

// GetTotalEntries returns the total number of entries across all pages
func (p *Paginator) GetTotalEntries() int {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return int(p.config.PageSize) * p.pageCount
}
