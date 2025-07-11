package ldap

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// TestClientConnection tests basic LDAP client connection functionality
func TestClientConnection(t *testing.T) {
	logger := zaptest.NewLogger(t)

	config := &Config{
		Host:     "mock.ldap.server",
		Port:     389,
		Username: "testuser",
		Password: "testpass",
		Timeout:  30 * time.Second,
		Logger:   logger,
	}

	client := NewClient(config)
	require.NotNil(t, client)
	assert.False(t, client.IsConnected())
}

// TestPaginatorBasic tests basic paginator functionality
func TestPaginatorBasic(t *testing.T) {
	logger := zaptest.NewLogger(t)

	config := &Config{
		Host:     "mock.ldap.server",
		Port:     389,
		Username: "testuser",
		Password: "testpass",
		Logger:   logger,
	}

	client := NewClient(config)
	require.NotNil(t, client)

	paginatorConfig := &PaginatorConfig{
		BaseDN:     "DC=example,DC=com",
		Filter:     "(objectClass=person)",
		Attributes: []string{"cn", "mail"},
		PageSize:   100,
		Logger:     logger,
	}

	paginator := NewPaginator(client, paginatorConfig)
	require.NotNil(t, paginator)
	assert.True(t, paginator.HasMore())
	assert.Equal(t, 0, paginator.GetPageCount())
}

// TestUserParser tests user parsing functionality
func TestUserParser(t *testing.T) {
	logger := zaptest.NewLogger(t)
	parser := NewUserParser(logger)
	require.NotNil(t, parser)

	// Test validation
	user := &User{
		DN:       "CN=testuser,DC=example,DC=com",
		Username: "testuser",
	}

	err := parser.ValidateUser(user)
	assert.NoError(t, err)

	// Test invalid user
	invalidUser := &User{
		DN: "CN=testuser,DC=example,DC=com",
	}

	err = parser.ValidateUser(invalidUser)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "username is required")
}

// TestHealthCheck tests the health check functionality
func TestHealthCheck(t *testing.T) {
	logger := zaptest.NewLogger(t)

	config := &Config{
		Host:     "mock.ldap.server",
		Port:     389,
		Username: "testuser",
		Password: "testpass",
		Logger:   logger,
	}

	client := NewClient(config)
	require.NotNil(t, client)

	// Should fail since we're not connected
	err := client.HealthCheck()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}
