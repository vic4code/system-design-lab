package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/vic4code/system-design-lab/beatstream/internal/db"
	"github.com/vic4code/system-design-lab/beatstream/internal/worker"
)

func main() {
	brokers := mustEnv("KAFKA_BROKERS")
	dbURL := mustEnv("DATABASE_URL")
	kafkaAuth := getEnv("KAFKA_AUTH", "plain")
	awsRegion := getEnv("AWS_REGION", "us-east-1")

	pool, err := db.Connect(dbURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	if err := db.Migrate(pool); err != nil {
		log.Fatalf("db migrate: %v", err)
	}

	uploadWorker, err := worker.NewUploadWorker(brokers, kafkaAuth, awsRegion, pool)
	if err != nil {
		log.Fatalf("upload worker: %v", err)
	}
	defer uploadWorker.Close()

	analyticsWorker, err := worker.NewAnalyticsWorker(brokers, kafkaAuth, awsRegion, pool)
	if err != nil {
		log.Fatalf("analytics worker: %v", err)
	}
	defer analyticsWorker.Close()

	ctx, cancel := context.WithCancel(context.Background())

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		log.Println("worker: shutting down...")
		cancel()
	}()

	go uploadWorker.Run(ctx)
	analyticsWorker.Run(ctx)
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required env var %s is not set", key)
	}
	return v
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
