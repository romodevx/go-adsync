package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	adsync "github.com/romodevx/go-adsync"
)

func main() {
	runRealTest()
}

func runRealTest() { //nolint:cyclop,funlen // Example functions can have higher complexity and length
	fmt.Println("🔗 go-adsync Real LDAP Test")
	fmt.Println("===========================")

	// Configuration for local LDAP server (Docker)
	config := &adsync.Config{
		Host:          "localhost",
		Port:          389,
		Username:      "cn=admin,dc=example,dc=com",
		Password:      "adminpassword",
		BaseDN:        "ou=users,dc=example,dc=com",
		UseSSL:        false,
		SkipTLSVerify: true,
		PageSize:      2, // Small page to see pagination
		Filter:        "(objectClass=inetOrgPerson)",
		Attributes:    []string{"cn", "sn", "givenName", "mail", "uid", "displayName"},
		SessionFile:   "ldap_session.json",
		StateStorage:  nil,
		Timeout:       30 * time.Second,
		RetryCount:    3,
		RetryDelay:    5 * time.Second,
		Logger:        nil,
		LogLevel:      "debug",
	}

	fmt.Printf("📋 Configuration:\n")
	fmt.Printf("   Host: %s:%d\n", config.Host, config.Port)
	fmt.Printf("   Username: %s\n", config.Username)
	fmt.Printf("   BaseDN: %s\n", config.BaseDN)
	fmt.Printf("   PageSize: %d\n", config.PageSize)
	fmt.Printf("   Filter: %s\n", config.Filter)

	// Create syncer
	syncer, err := adsync.New(config)
	if err != nil {
		log.Fatalf("❌ Error creating syncer: %v", err)
	}
	defer syncer.Close()

	fmt.Println("\n✅ Syncer created successfully")

	// Configure signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		fmt.Println("\n🛑 Signal received, stopping synchronization...")
		cancel()
	}()

	// Test 1: Complete synchronization
	fmt.Println("\n🔄 Test 1: Complete Synchronization")
	fmt.Println("==================================")

	startTime := time.Now()
	userChan, errChan := syncer.SyncAll(ctx)
	userCount := 0

	for {
		select {
		case user, ok := <-userChan:
			if !ok {
				userChan = nil
				continue
			}
			userCount++
			fmt.Printf("👤 User %d:\n", userCount)
			fmt.Printf("   DN: %s\n", user.DN)
			fmt.Printf("   Username: %s\n", user.Username)
			fmt.Printf("   Email: %s\n", user.Email)
			fmt.Printf("   Display Name: %s\n", user.DisplayName)
			fmt.Printf("   Attributes: %d\n", len(user.Attributes))
			fmt.Println()

			// Show statistics every 2 users
			if userCount%2 == 0 {
				stats := syncer.GetStats()
				fmt.Printf("📊 Partial Statistics:\n")
				fmt.Printf("   Duration: %v\n", stats.Duration)
				fmt.Printf("   Users/second: %.2f\n", stats.UsersPerSecond)
				fmt.Println()
			}

		case err, ok := <-errChan:
			if !ok {
				errChan = nil
				continue
			}
			if err != nil {
				fmt.Printf("⚠️  Error: %v\n", err)
			}

		case <-ctx.Done():
			fmt.Println("🛑 Synchronization canceled")
			goto finish //nolint:nlreturn // Acceptable in examples
		}

		if userChan == nil && errChan == nil {
			break
		}
	}

finish:
	duration := time.Since(startTime)

	// Final statistics
	fmt.Println("\n📈 Final Statistics")
	fmt.Println("======================")

	stats := syncer.GetStats()
	result := syncer.GetResult()

	fmt.Printf("⏱️  Total duration: %v\n", duration)
	fmt.Printf("👥 Users processed: %d\n", result.ProcessedUsers)
	fmt.Printf("⚠️  Users skipped: %d\n", result.SkippedUsers)
	fmt.Printf("❌ Errors: %d\n", result.ErrorCount)
	fmt.Printf("📄 Pages processed: %d\n", stats.TotalPages)
	fmt.Printf("🚀 Average speed: %.2f users/sec\n", stats.UsersPerSecond)

	if result.ErrorCount > 0 {
		fmt.Printf("\n🔍 Error details:\n")
		for i, err := range result.Errors {
			fmt.Printf("   %d. %v\n", i+1, err)
		}
	}

	fmt.Println("\n🎉 Test completed!")
	fmt.Println("\n💡 Next steps:")
	fmt.Println("   1. Check the 'ldap_session.json' file to see the saved state")
	fmt.Println("   2. Run again to test Resume()")
	fmt.Println("   3. Modify PageSize to see different behaviors")
}
