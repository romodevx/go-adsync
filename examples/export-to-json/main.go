package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	adsync "github.com/romodevx/go-adsync"
)

func main() {
	fmt.Println("go-adsync JSON Export Example (memory only)")
	fmt.Println("==========================================")

	config := &adsync.Config{
		Host:          "localhost", // Change to your LDAP server
		Port:          389,
		Username:      "cn=admin,dc=example,dc=com",
		Password:      "adminpassword",
		BaseDN:        "ou=users,dc=example,dc=com",
		UseSSL:        false,
		SkipTLSVerify: true,
		PageSize:      100,
		Filter:        "(objectClass=inetOrgPerson)",
		Attributes:    []string{"cn", "sn", "givenName", "mail", "uid", "displayName"},
		Timeout:       30 * time.Second,
		RetryCount:    3,
		RetryDelay:    5 * time.Second,
		LogLevel:      "info",
		// No StateStorage: only memory
	}

	syncer, err := adsync.New(config)
	if err != nil {
		log.Fatalf("Error creating syncer: %v", err)
	}
	defer syncer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	userChan, errChan := syncer.SyncAll(ctx)

	outputFile := "exported_users.json"
	file, err := os.Create(outputFile)
	if err != nil {
		log.Fatalf("Error creating output file: %v", err)
	}
	defer file.Close()

	file.WriteString("[\n")
	first := true
	userCount := 0
	for userChan != nil || errChan != nil {
		select {
		case user, ok := <-userChan:
			if !ok {
				userChan = nil
				continue
			}
			userCount++
			if !first {
				file.WriteString(",\n")
			}
			first = false
			data, _ := json.MarshalIndent(user, "  ", "  ")
			file.Write(data)
			if userCount%10 == 0 {
				fmt.Printf("Exported %d users...\n", userCount)
			}
		case err, ok := <-errChan:
			if !ok {
				errChan = nil
				continue
			}
			if err != nil {
				fmt.Printf("Error: %v\n", err)
			}
		}
	}
	file.WriteString("\n]\n")
	fmt.Printf("Exported %d users to %s\n", userCount, outputFile)
}
