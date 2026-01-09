// Package main provides a migration tool for encrypting existing plaintext SecretKeys.
//
// Usage:
//
//	go run cmd/migrate/main.go -master-key <base64-master-key> -redis <redis-address>
//	go run cmd/migrate/main.go -generate-key
//
// This tool migrates all existing clients from plaintext SecretKey storage to
// AES-256-GCM encrypted storage. After migration:
// - SecretKeyEncrypted field contains the encrypted SecretKey
// - SecretKey field is cleared (no plaintext storage)
// - SecretKeyVersion is set to 1 (if not already set)
//
// Note: This is a one-way migration. The plaintext SecretKey will be lost after migration.
// Make sure to backup your data before running this tool.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/core/storage"
	"tunnox-core/internal/security"
)

func main() {
	// Check for generate-key command first
	if len(os.Args) > 1 && os.Args[1] == "-generate-key" {
		key, err := security.GenerateMasterKey()
		if err != nil {
			fmt.Printf("Error generating master key: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Generated master key (save this securely):\n%s\n", key)
		return
	}

	// Parse command line flags
	masterKey := flag.String("master-key", "", "Base64 encoded 32-byte master key for AES-256-GCM encryption")
	redisAddr := flag.String("redis", "localhost:6379", "Redis server address")
	redisPassword := flag.String("redis-password", "", "Redis password")
	redisDB := flag.Int("redis-db", 0, "Redis database number")
	dryRun := flag.Bool("dry-run", false, "Perform a dry run without making changes")
	flag.Parse()

	if *masterKey == "" {
		fmt.Println("Error: -master-key is required")
		fmt.Println()
		fmt.Println("Usage:")
		fmt.Println("  go run cmd/migrate/main.go -generate-key")
		fmt.Println("  go run cmd/migrate/main.go -master-key <key> [-redis <addr>] [-dry-run]")
		os.Exit(1)
	}

	fmt.Println("=== SecretKey Migration Tool ===")
	fmt.Printf("Redis: %s\n", *redisAddr)
	fmt.Printf("Dry run: %v\n", *dryRun)
	fmt.Println()

	// Create context
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Create SecretKeyManager
	secretKeyConfig := &security.SecretKeyConfig{
		MasterKey: *masterKey,
	}
	secretKeyMgr, err := security.NewSecretKeyManager(secretKeyConfig)
	if err != nil {
		fmt.Printf("Error creating SecretKeyManager: %v\n", err)
		os.Exit(1)
	}

	// Connect to Redis
	redisConfig := &storage.RedisConfig{
		Addr:     *redisAddr,
		Password: *redisPassword,
		DB:       *redisDB,
		PoolSize: 10,
	}
	redisStorage, err := storage.NewRedisStorage(ctx, redisConfig)
	if err != nil {
		fmt.Printf("Error connecting to Redis: %v\n", err)
		os.Exit(1)
	}
	defer redisStorage.Close()

	// Create repository
	repo := repos.NewRepository(redisStorage)
	configRepo := repos.NewClientConfigRepository(repo)

	// Get all client configs
	fmt.Println("Fetching all client configs...")
	configs, err := configRepo.ListConfigs()
	if err != nil {
		fmt.Printf("Error listing configs: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d clients\n", len(configs))
	fmt.Println()

	// Statistics
	var migrated, skipped, errors int

	for _, config := range configs {
		fmt.Printf("Client %d: ", config.ID)

		// Check if already migrated
		if config.SecretKeyEncrypted != "" {
			fmt.Println("SKIPPED (already encrypted)")
			skipped++
			continue
		}

		// Check if has plaintext key
		if config.SecretKey == "" {
			fmt.Println("SKIPPED (no SecretKey)")
			skipped++
			continue
		}

		if *dryRun {
			fmt.Println("WOULD MIGRATE (dry run)")
			migrated++
			continue
		}

		// Encrypt the plaintext key
		encrypted, err := secretKeyMgr.Encrypt(config.SecretKey)
		if err != nil {
			fmt.Printf("ERROR: %v\n", err)
			errors++
			continue
		}

		// Update config
		config.SecretKeyEncrypted = encrypted
		if config.SecretKeyVersion == 0 {
			config.SecretKeyVersion = 1
		}
		config.SecretKey = "" // Clear plaintext
		config.UpdatedAt = time.Now()

		// Save config
		if err := configRepo.UpdateConfig(config); err != nil {
			fmt.Printf("ERROR saving: %v\n", err)
			errors++
			continue
		}

		fmt.Println("MIGRATED")
		migrated++
	}

	// Print summary
	fmt.Println()
	fmt.Println("=== Migration Summary ===")
	fmt.Printf("Total clients: %d\n", len(configs))
	fmt.Printf("Migrated: %d\n", migrated)
	fmt.Printf("Skipped: %d\n", skipped)
	fmt.Printf("Errors: %d\n", errors)

	if *dryRun {
		fmt.Println()
		fmt.Println("This was a dry run. No changes were made.")
		fmt.Println("Remove -dry-run flag to perform actual migration.")
	}

	if errors > 0 {
		os.Exit(1)
	}
}
