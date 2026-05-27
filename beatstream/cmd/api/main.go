package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/vic4code/system-design-lab/beatstream/internal/cache"
	"github.com/vic4code/system-design-lab/beatstream/internal/db"
	"github.com/vic4code/system-design-lab/beatstream/internal/handler"
	applogger "github.com/vic4code/system-design-lab/beatstream/internal/logger"
	"github.com/vic4code/system-design-lab/beatstream/internal/middleware"
	"github.com/vic4code/system-design-lab/beatstream/internal/queue"
	"github.com/vic4code/system-design-lab/beatstream/internal/storage"
)

func main() {
	// ── Structured logger ──────────────────────────────────────────────────────
	// LOG_FORMAT=json → newline-delimited JSON (CloudWatch)
	// LOG_FORMAT=console (default) → coloured human-readable (local dev)
	log := applogger.Init()
	defer applogger.Sync()

	ctx := context.Background()

	pool, err := db.Connect(mustEnv("DATABASE_URL"))
	if err != nil {
		log.Fatal("db connect", zap.Error(err))
	}
	defer pool.Close()

	if err := db.Migrate(pool); err != nil {
		log.Fatal("db migrate", zap.Error(err))
	}

	// ── S3-compatible storage (optional) ───────────────────────────────────────
	// Local dev: S3_ENDPOINT=http://minio:9000 + S3_ACCESS_KEY/S3_SECRET_KEY
	// AWS ECS:   leave S3_ENDPOINT unset — ECS task role provides credentials
	// S3_PRESIGN_ENDPOINT: browser-accessible URL for pre-signed audio links
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
			log.Fatal("storage init", zap.Error(err))
		}
		if os.Getenv("S3_ENDPOINT") != "" {
			if err := store.EnsureBucket(ctx); err != nil {
				log.Fatal("ensure bucket", zap.Error(err))
			}
		}
	} else {
		log.Info("S3_BUCKET not set — upload/stream disabled")
	}

	rdb, err := cache.NewRedis(mustEnv("REDIS_URL"))
	if err != nil {
		log.Fatal("redis connect", zap.Error(err))
	}

	// ── Kafka producer (optional) ───────────────────────────────────────────────
	// KAFKA_AUTH=iam → MSK SASL/IAM (production); unset → PLAINTEXT (local)
	var producer *queue.Producer
	if brokers := os.Getenv("KAFKA_BROKERS"); brokers != "" {
		if os.Getenv("KAFKA_AUTH") == "iam" {
			producer, err = queue.NewProducerIAM(brokers, mustEnv("AWS_REGION"))
		} else {
			producer, err = queue.NewProducer(brokers)
		}
		if err != nil {
			log.Fatal("kafka producer", zap.Error(err))
		}
		defer producer.Close()
	} else {
		log.Info("KAFKA_BROKERS not set — upload pipeline disabled")
	}

	// ── Gin engine ─────────────────────────────────────────────────────────────
	gin.SetMode(getEnv("GIN_MODE", "debug"))
	r := gin.New()

	// Middleware stack (order matters):
	// 1. Recovery    — catch panics, return 500
	// 2. RequestID   — stamp X-Request-ID on every request
	// 3. ZapLogger   — structured JSON request log (replaces gin.Logger)
	// 4. SecurityHeaders — defensive HTTP headers + tightened CORS
	// 5. PrometheusMetrics — Prometheus counters/histograms
	r.Use(
		gin.Recovery(),
		middleware.RequestID(),
		middleware.ZapLogger(),
		middleware.SecurityHeaders(),
		middleware.PrometheusMetrics(),
	)

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

	jwtSecret := getEnv("JWT_SECRET", "dev-secret-change-in-production")

	auth := handler.NewAuth(pool, jwtSecret)
	artists := handler.NewArtists(pool)
	tracks := handler.NewTracks(pool, store, rdb, producer)
	playlists := handler.NewPlaylists(pool)
	search := handler.NewSearch(pool, rdb)

	requireAuth := middleware.RequireAuth(jwtSecret)
	auditLog := middleware.AuditLog(pool)
	loginLimit := middleware.LoginRateLimit(rdb.Client())

	v1 := r.Group("/v1")
	v1.Use(auditLog) // Audit every write under /v1
	{
		// Auth — login route has brute-force protection
		v1.POST("/auth/register", auth.Register)
		v1.POST("/auth/login", loginLimit, auth.Login)
		v1.GET("/auth/me", requireAuth, auth.Me)

		// Artists (read public, write protected)
		v1.GET("/artists", artists.List)
		v1.POST("/artists", requireAuth, artists.Create)
		v1.GET("/artists/:id", artists.Get)

		// Tracks (read public, write protected)
		v1.GET("/tracks", tracks.List)
		v1.GET("/tracks/:id", tracks.Get)
		v1.POST("/tracks", requireAuth, tracks.Create)
		v1.GET("/tracks/:id/stream",
			middleware.StreamRateLimit(rdb.Client(), 100),
			tracks.Stream,
		)

		// Playlists (read public, write protected)
		v1.GET("/playlists", playlists.List)
		v1.GET("/playlists/:id", playlists.Get)
		v1.POST("/playlists", requireAuth, playlists.Create)
		v1.GET("/playlists/:id/tracks", playlists.ListTracks)
		v1.POST("/playlists/:id/tracks", requireAuth, playlists.AddTrack)
		v1.DELETE("/playlists/:id/tracks/:track_id", requireAuth, playlists.RemoveTrack)

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
		log.Info("beatstream API started", zap.String("port", port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("server", zap.Error(err))
		}
	}()
	go func() {
		log.Info("metrics server started", zap.String("port", metricsPort))
		if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Warn("metrics server stopped", zap.Error(err))
		}
	}()

	<-quit
	log.Info("shutting down gracefully...")

	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(shutCtx)
	metricsSrv.Shutdown(shutCtx)

	log.Info("shutdown complete")
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		applogger.L().Fatal("required env var not set", zap.String("key", key))
	}
	return v
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
