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
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.uber.org/zap"

	"github.com/vic4code/system-design-lab/beatstream/internal/cache"
	"github.com/vic4code/system-design-lab/beatstream/internal/db"
	"github.com/vic4code/system-design-lab/beatstream/internal/handler"
	applogger "github.com/vic4code/system-design-lab/beatstream/internal/logger"
	"github.com/vic4code/system-design-lab/beatstream/internal/middleware"
	"github.com/vic4code/system-design-lab/beatstream/internal/queue"
	"github.com/vic4code/system-design-lab/beatstream/internal/search"
	"github.com/vic4code/system-design-lab/beatstream/internal/storage"
	"github.com/vic4code/system-design-lab/beatstream/internal/telemetry"
)

func main() {
	// ── Structured logger ──────────────────────────────────────────────────────
	// LOG_FORMAT=json → newline-delimited JSON (CloudWatch)
	// LOG_FORMAT=console (default) → coloured human-readable (local dev)
	log := applogger.Init()
	defer applogger.Sync()

	ctx := context.Background()

	// ── OpenTelemetry tracing ──────────────────────────────────────────────────
	// Third pillar of observability: Logs (zap) + Metrics (Prometheus) + Traces (OTel)
	// OTEL_EXPORTER_OTLP_ENDPOINT: http://jaeger:4318 (local) or ADOT sidecar (AWS)
	// Traces flow: gin handler → child spans for DB/Redis → exported via OTLP
	otelShutdown, err := telemetry.Init(ctx)
	if err != nil {
		log.Warn("OpenTelemetry init failed — tracing disabled", zap.Error(err))
	} else {
		defer otelShutdown(ctx)
		log.Info("OpenTelemetry tracing enabled",
			zap.String("endpoint", func() string {
				if ep := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); ep != "" {
					return ep
				}
				return "http://localhost:4318"
			}()),
		)
	}

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
	// 1. Recovery         — catch panics, return 500
	// 2. OtelGin          — create root OTel span per request (must be early)
	// 3. RequestID        — stamp X-Request-ID (reuse OTel trace_id if available)
	// 4. ZapLogger        — structured JSON log with trace_id + request_id correlation
	// 5. SecurityHeaders  — defensive HTTP headers + CORS
	// 6. PrometheusMetrics — Prometheus counters/histograms
	r.Use(
		gin.Recovery(),
		otelgin.Middleware("beatstream-api"), // Creates root span; propagates W3C traceparent
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

	// ── OpenSearch (optional) ──────────────────────────────────────────────────
	// OPENSEARCH_URL: http://opensearch:9200 (local) or VPC endpoint (AWS)
	// When available, search uses fuzzy multi-field matching instead of tsvector.
	var osClient *search.Client
	if osURL := os.Getenv("OPENSEARCH_URL"); osURL != "" {
		osClient = search.New(osURL)
		if err := osClient.EnsureIndex(ctx); err != nil {
			log.Warn("OpenSearch index setup failed — falling back to PostgreSQL", zap.Error(err))
			osClient = nil
		} else {
			log.Info("OpenSearch connected", zap.String("endpoint", osURL))
		}
	}

	jwtSecret := getEnv("JWT_SECRET", "dev-secret-change-in-production")

	auth := handler.NewAuth(pool, jwtSecret)
	artists := handler.NewArtists(pool)
	tracks := handler.NewTracks(pool, store, rdb, producer)
	playlists := handler.NewPlaylists(pool)
	searchHandler := handler.NewSearch(pool, rdb, osClient)
	recs := handler.NewRecommendations(pool, rdb)
	favorites := handler.NewFavorites(pool)
	admin := handler.NewAdmin(pool)

	requireAuth := middleware.RequireAuth(jwtSecret)
	requireAdmin := middleware.RequireRole("admin")
	auditLog := middleware.AuditLog(pool)
	loginLimit := middleware.LoginRateLimit(rdb.Client())

	v1 := r.Group("/v1")
	v1.Use(auditLog) // Audit every write under /v1
	{
		// Auth — login route has brute-force protection
		v1.POST("/auth/register", auth.Register)
		v1.POST("/auth/login", loginLimit, auth.Login)
		v1.GET("/auth/me", requireAuth, auth.Me)

		// GDPR
		v1.DELETE("/me", requireAuth, auth.DeleteMe)
		v1.GET("/me/export", requireAuth, auth.ExportMe)

		// Admin (RBAC: admin role required)
		adminGroup := v1.Group("/admin", requireAuth, requireAdmin)
		{
			adminGroup.GET("/users", admin.ListUsers)
			adminGroup.DELETE("/users/:id", admin.DeleteUser)
			adminGroup.GET("/audit-logs", admin.ListAuditLogs)
		}

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
		v1.DELETE("/playlists/:id", requireAuth, playlists.Delete)
		v1.GET("/playlists/:id/tracks", playlists.ListTracks)
		v1.POST("/playlists/:id/tracks", requireAuth, playlists.AddTrack)
		v1.DELETE("/playlists/:id/tracks/:track_id", requireAuth, playlists.RemoveTrack)

		v1.GET("/search", searchHandler.Search)
		v1.GET("/search/autocomplete", searchHandler.Autocomplete)

		// Favorites
		v1.GET("/favorites", requireAuth, favorites.List)
		v1.GET("/favorites/ids", requireAuth, favorites.IDs)
		v1.POST("/favorites", requireAuth, favorites.Add)
		v1.DELETE("/favorites/:track_id", requireAuth, favorites.Remove)

		// Recommendations
		v1.GET("/recommendations/top", recs.TopTracks)
		v1.GET("/recommendations/for-you", requireAuth, recs.ForYou)
		v1.GET("/tracks/:id/similar", recs.Similar)
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
