# go-adsync
Go library for robust Active Directory synchronization with LDAP session persistence, configurable pagination, and automatic recovery from connection failures.

## Features

- **Memory-first approach**: Uses in-memory storage by default for better security
- **LDAP session persistence**: Resume interrupted operations from saved state (in memory)
- **Configurable pagination**: Handle large datasets efficiently
- **Automatic recovery**: Retry logic and connection management
- **Comprehensive logging**: Structured logging with configurable levels
- **Thread-safe operations**: Safe for concurrent use

## Installation

```bash
go get github.com/romodevx/go-adsync
```

## Quick Start

### Basic Usage (Memory Storage)

```go
package main

import (
    "context"
    "fmt"
    "time"
    
    adsync "github.com/romodevx/go-adsync"
)

func main() {
    // Create configuration
    config := &adsync.Config{
        Host:          "ldap.company.com",
        Port:          389,
        Username:      "service-account",
        Password:      "password",
        BaseDN:        "DC=company,DC=com",
        PageSize:      1000,
        Filter:        "(objectClass=person)",
        Attributes:    []string{"cn", "mail", "sAMAccountName"},
        Timeout:       30 * time.Second,
        RetryCount:    3,
        RetryDelay:    5 * time.Second,
        LogLevel:      "info",
    }

    // Create syncer (uses memory storage by default)
    syncer, err := adsync.New(config)
    if err != nil {
        panic(err)
    }
    defer syncer.Close()

    // Sync all users
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
    defer cancel()

    userChan, errChan := syncer.SyncAll(ctx)

    for {
        select {
        case user, ok := <-userChan:
            if !ok {
                userChan = nil
                continue
            }
            fmt.Printf("User: %s, Email: %s\n", user.Username, user.Email)

        case err, ok := <-errChan:
            if !ok {
                errChan = nil
                continue
            }
            fmt.Printf("Error: %v\n", err)
        }

        if userChan == nil && errChan == nil {
            break
        }
    }

    // Get statistics
    stats := syncer.GetStats()
    fmt.Printf("Users processed: %d\n", stats.TotalPages * config.PageSize)
}
```

## Examples

Check the `examples/` directory for complete working examples:

- **`export-to-json/`**: Export users to JSON file
- **`real-test/`**: Test with real LDAP server
- **`mock-test/`**: Test with mock data
- **`docker-ldap/`**: Local LDAP server for testing

## Security

- **Memory-first**: No files created by default
- **No automatic file creation**: Users must explicitly opt-in to file storage (not included by default)

## API Reference

### Configuration

```go
type Config struct {
    Host          string        // LDAP server hostname
    Port          int           // LDAP server port (default: 389/636)
    Username      string        // LDAP bind username
    Password      string        // LDAP bind password
    BaseDN        string        // Search base DN
    UseSSL        bool          // Use SSL/TLS
    SkipTLSVerify bool          // Skip TLS certificate verification
    PageSize      int           // Pagination size (default: 1000)
    Filter        string        // LDAP search filter
    Attributes    []string      // Attributes to retrieve
    Timeout       time.Duration // Connection timeout
    RetryCount    int           // Retry attempts
    RetryDelay    time.Duration // Delay between retries
    Logger        *zap.Logger   // Custom logger
    LogLevel      string        // Log level (debug, info, warn, error)
}
```

### Main Interface

```go
type Syncer interface {
    // SyncAll performs complete synchronization
    SyncAll(ctx context.Context) (<-chan User, <-chan error)
    
    // SyncIncremental syncs changes since given time
    SyncIncremental(ctx context.Context, since time.Time) (<-chan User, <-chan error)
    
    // Resume continues from saved state (in memory)
    Resume(ctx context.Context) (<-chan User, <-chan error)
    
    // GetStats returns current statistics
    GetStats() SyncStats
    
    // GetResult returns final result
    GetResult() SyncResult
    
    // Reset clears saved state (in memory)
    Reset() error
    
    // Close releases resources
    Close() error
}
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

MIT License - see LICENSE file for details.
