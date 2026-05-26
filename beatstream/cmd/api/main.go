package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/vic4code/system-design-lab/beatstream/internal/cache"
	"github.com/vic4code/system-design-lab/beatstream/internal/db"
	"github.com/vic4code/system-design-lab/beatstream/internal/handler"
	"github.com/vic4code/system-design-lab/beatstream/internal/middleware"
	"github.com/vic4code/system-design-lab/beatstream/internal/queue"
	"github.com/vic4code/system-design-lab/beatstream/internal/storage"
)

func main() {
	ctx := context.Background()

	pool, err := db.Connect(mustEnv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	if err := db.Migrate(pool); err != nil {
		log.Fatalf("db migrate: %v", err)
	}

	// S3-compatible storage.
	// Local dev: set S3_ENDPOINT=http://minio:9000 + S3_ACCESS_KEY/S3_SECRET_KEY.
	// AWS ECS:   leave S3_ENDPOINT unset — the task role provides credentials.
	// S3_PRESIGN_ENDPOINT: browser-accessible URL for pre-signed audio links.
	//   In docker-compose set to http://localhost:9000 so browsers can reach MinIO.
	// S3-compatible storage — optional. If S3_BUCKET is unset, upload and
	// stream endpoints are disabled but all read endpoints remain functional.
	var store *storage.Storage
	if bucket := os.Getenv("S3_BUCKET"); bucket != "" {
		store, err = storage.New(ctx, storage.Config{
			Bucket:          bucket,
			Region:          getEnv("AWS_REGION", "us-east-1"),
			Endpoint:        os.Getenv("S3_ENDPOINT"),
			AccessKey:       os.Getenv("S3_ACCESS_KEY"),
			SecretKey:       os.Getenv("S3_SECRET_KEY"),
			PresignEndpoint: os.Getenv("S3_PRESIGN_ENDPOINT"),
		})
		if err != nil {
			log.Fatalf("storage init: %v", err)
		}
		if os.Getenv("S3_ENDPOINT") != "" {
			if err := store.EnsureBucket(ctx); err != nil {
				log.Fatalf("ensure bucket: %v", err)
			}
		}
	} else {
		log.Println("S3_BUCKET not set — upload/stream disabled")
	}

	rdb, err := cache.NewRedis(mustEnv("REDIS_URL"))
	if err != nil {
		log.Fatalf("redis connect: %v", err)
	}

	// Kafka producer — optional. If KAFKA_BROKERS is unset the upload
	// pipeline is disabled but all read endpoints remain functional.
	var producer *queue.Producer
	if brokers := os.Getenv("KAFKA_BROKERS"); brokers != "" {
		if os.Getenv("KAFKA_AUTH") == "iam" {
			producer, err = queue.NewProducerIAM(brokers, mustEnv("AWS_REGION"))
		} else {
			producer, err = queue.NewProducer(brokers)
		}
		if err != nil {
			log.Fatalf("kafka producer: %v", err)
		}
		defer producer.Close()
	} else {
		log.Println("KAFKA_BROKERS not set — upload pipeline disabled")
	}

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), middleware.CORS(), middleware.RequestID(), middleware.PrometheusMetrics())

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/ready", func(c *gin.Context) {
		if err := pool.Ping(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "db unavailable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})

	artists := handler.NewArtists(pool)
	tracks := handler.NewTracks(pool, store, rdb, producer)
	playlists := handler.NewPlaylists(pool)
	search := handler.NewSearch(pool, rdb)

	v1 := r.Group("/v1")
	{
		v1.GET("/artists", artists.List)
		v1.POST("/artists", artists.Create)
		v1.GET("/artists/:id", artists.Get)

		v1.GET("/tracks", tracks.List)

		v1.GET("/tracks/:id", tracks.Get)
		v1.POST("/tracks", tracks.Create)
		v1.GET("/tracks/:id/stream",
			middleware.StreamRateLimit(rdb.Client(), 100),
			tracks.Stream,
		)

		v1.GET("/playlists", playlists.List)
		v1.GET("/playlists/:id", playlists.Get)
		v1.POST("/playlists", playlists.Create)
		v1.GET("/playlists/:id/tracks", playlists.ListTracks)
		v1.POST("/playlists/:id/tracks", playlists.AddTrack)
		v1.DELETE("/playlists/:id/tracks/:track_id", playlists.RemoveTrack)

		v1.GET("/search", search.Search)
	}

	port := getEnv("PORT", "8080")
	srv := &http.Server{Addr: ":" + port, Handler: r}

	metricsPort := getEnv("METRICS_PORT", "9090")
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())
	metricsSrv := &http.Server{Addr: ":" + metricsPort, Handler: metricsMux}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("beatstream API listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()
	go func() {
		log.Printf("metrics server listening on :%s/metrics", metricsPort)
		if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("metrics server: %v", err)
		}
	}()

	<-quit
	log.Println("shutting down...")

	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(shutCtx)
	metricsSrv.Shutdown(shutCtx)
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
