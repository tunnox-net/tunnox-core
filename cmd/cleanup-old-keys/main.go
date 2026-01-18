package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	redisstorage "tunnox-core/internal/core/storage/redis"
)

var patternsToDelete = []string{
	"tunnox:id:used:conn:*",
	"tunnox:id:used:tunnel:*",
	"tunnox:id:used:portmapping:instance:*",
}

func main() {
	redisAddr := flag.String("redis", "localhost:6379", "Redis server address")
	redisPassword := flag.String("redis-password", "", "Redis password")
	redisDB := flag.Int("redis-db", 0, "Redis database number")
	dryRun := flag.Bool("dry-run", true, "Perform a dry run without making changes (default: true)")
	batchSize := flag.Int("batch-size", 1000, "Number of keys to delete per batch")
	confirm := flag.Bool("confirm", false, "Skip confirmation prompt (use with caution)")
	flag.Parse()

	fmt.Println("=== Storage Refactor Cleanup Tool ===")
	fmt.Println("Removes orphaned ID tracking keys from old storage scheme")
	fmt.Println()
	fmt.Printf("Redis: %s\n", *redisAddr)
	fmt.Printf("Dry run: %v\n", *dryRun)
	fmt.Printf("Batch size: %d\n", *batchSize)
	fmt.Println()

	if !*dryRun && !*confirm {
		fmt.Print("WARNING: This will DELETE keys from Redis. Type 'yes' to continue: ")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		if strings.TrimSpace(input) != "yes" {
			fmt.Println("Aborted.")
			os.Exit(0)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	redisConfig := &redisstorage.Config{
		Addr:     *redisAddr,
		Password: *redisPassword,
		DB:       *redisDB,
		PoolSize: 10,
	}
	storage, err := redisstorage.New(ctx, redisConfig)
	if err != nil {
		fmt.Printf("Error connecting to Redis: %v\n", err)
		os.Exit(1)
	}
	defer storage.Close()

	var totalDeleted int64

	for _, pattern := range patternsToDelete {
		fmt.Printf("\n--- Processing pattern: %s ---\n", pattern)
		deleted, err := deleteKeysByPattern(ctx, storage, pattern, *batchSize, *dryRun)
		if err != nil {
			fmt.Printf("Error processing pattern %s: %v\n", pattern, err)
			continue
		}
		totalDeleted += deleted
		fmt.Printf("Pattern %s: %d keys %s\n", pattern, deleted, actionWord(*dryRun))
	}

	fmt.Println()
	fmt.Println("=== Cleanup Summary ===")
	fmt.Printf("Total keys %s: %d\n", actionWord(*dryRun), totalDeleted)

	if *dryRun {
		fmt.Println()
		fmt.Println("This was a dry run. No changes were made.")
		fmt.Println("Use -dry-run=false -confirm to perform actual deletion.")
	}
}

func actionWord(dryRun bool) string {
	if dryRun {
		return "would be deleted"
	}
	return "deleted"
}

func deleteKeysByPattern(ctx context.Context, storage *redisstorage.Storage, pattern string, batchSize int, dryRun bool) (int64, error) {
	client := storage.Client()

	var cursor uint64
	var totalDeleted int64

	for {
		keys, nextCursor, err := client.Scan(ctx, cursor, pattern, int64(batchSize)).Result()
		if err != nil {
			return totalDeleted, fmt.Errorf("scan failed: %w", err)
		}

		if len(keys) > 0 {
			if dryRun {
				fmt.Printf("  Would delete %d keys (sample: %s)\n", len(keys), keys[0])
				totalDeleted += int64(len(keys))
			} else {
				deleted, err := client.Unlink(ctx, keys...).Result()
				if err != nil {
					return totalDeleted, fmt.Errorf("unlink failed: %w", err)
				}
				totalDeleted += deleted
				fmt.Printf("  Unlinked %d keys\n", deleted)
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return totalDeleted, nil
}
