package main

import (
	"context"
	"fmt"
	"time"

	adsync "github.com/romodevx/go-adsync"
)

func main() {
	runMockTests()
}

func runMockTests() { //nolint:cyclop,funlen // Example functions can have higher complexity and length
	fmt.Println("go-adsync Mock Test")
	fmt.Println("===================")

	// Test 1: Create basic configuration
	fmt.Println("\nTest 1: Basic configuration")
	config := adsync.DefaultConfig()
	config.Host = "mock.ldap.server"
	config.Username = "testuser"
	config.Password = "testpass"
	config.BaseDN = "DC=example,DC=com"

	err := config.Validate()
	if err != nil {
		fmt.Printf("Validation error: %v\n", err)
	} else {
		fmt.Printf("Valid configuration\n")
		fmt.Printf("   Host: %s\n", config.Host)
		fmt.Printf("   PageSize: %d\n", config.PageSize)
		fmt.Printf("   Filter: %s\n", config.Filter)
	}

	// Test 2: Create syncer (will fail connection but show structure)
	fmt.Println("\nTest 2: Create Syncer")
	syncer, err := adsync.New(config)
	if err != nil {
		fmt.Printf("Error creating syncer: %v\n", err)
		return
	}
	fmt.Printf("Syncer created successfully\n")
	defer syncer.Close()

	// Test 3: Test connection (we expect it to fail)
	fmt.Println("\nTest 3: Connection Test (Mock)")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fmt.Printf("Attempting synchronization (should fail with mock server)...\n")
	userChan, errChan := syncer.SyncAll(ctx)

	// Monitor channels for a few seconds
	timeout := time.After(3 * time.Second)
	userCount := 0

	for {
		select {
		case user, ok := <-userChan:
			if !ok {
				userChan = nil
				continue
			}
			userCount++
			fmt.Printf("User received: %s\n", user.Username)

		case err, ok := <-errChan:
			if !ok {
				errChan = nil
				continue
			}
			if err != nil {
				fmt.Printf("Expected error (mock server): %v\n", err)
				goto cleanup //nolint:nlreturn // Acceptable in examples
			}

		case <-timeout:
			fmt.Printf("Timeout reached\n")
			goto cleanup //nolint:nlreturn // Acceptable in examples
		}

		if userChan == nil && errChan == nil {
			break
		}
	}

cleanup:
	fmt.Printf("Users processed: %d\n", userCount)

	// Test 4: Statistics
	fmt.Println("\nTest 4: Statistics")
	stats := syncer.GetStats()
	result := syncer.GetResult()

	fmt.Printf("Statistics obtained:\n")
	fmt.Printf("   Duration: %v\n", stats.Duration)
	fmt.Printf("   Users processed: %d\n", result.ProcessedUsers)
	fmt.Printf("   Errors: %d\n", result.ErrorCount)

	fmt.Println("\nTests completed!")
	fmt.Println("To test with a real LDAP server:")
	fmt.Println("   1. Modify config.Host with your LDAP server")
	fmt.Println("   2. Configure valid credentials")
	fmt.Println("   3. Adjust BaseDN according to your domain")
}
