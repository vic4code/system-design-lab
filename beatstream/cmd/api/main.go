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
	"github.com/vic4code/system-design-lab/beatstream/internal/db"
	"github.com/vic4code/system-design-lab/beatstream/internal/handler"
	"github.com/vic4code/system-design-lab/beatstream/internal/middleware"
	"github.com/vic4code/system-design-lab/beatstream/internal/storage"
)

func main() {
	// Connect to PostgreSQL
	pool, err := db.Connect(mustEnv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	// Run migrations on startup
	if err := db.Migrate(pool); err != nil {
		log.Fatalf("db migrate: %v", err)
	}

	// Connect to MinIO (S3-compatible)
	store, err := storage.NewMinIO(storage.Config{
		Endpoint:  mustEnv("MINIO_ENDPOINT"),
		AccessKey: mustEnv("MINIO_ACCESS_KEY"),
		SecretKey: mustEnv("MINIO_SECRET_KEY"),
		Bucket:    mustEnv("MINIO_BUCKET"),
	})
	if err != nil {
		log.Fatalf("minio connect: %v", err)
	}

	// Ensure bucket exists
	if err := store.EnsureBucket(context.Background()); err != nil {
		log.Fatalf("minio ensure bucket: %v", err)
	}

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), middleware.RequestID())

	// Health endpoints
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/ready", func(c *gin.Context) {
		if err := pool.Ping(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "db unavailable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})

	// v1 API
	tracks := handler.NewTracks(pool, store)
	playlists := handler.NewPlaylists(pool)
	search := handler.NewSearch(pool)

	v1 := r.Group("/v1")
	{
		v1.GET("/tracks/:id", tracks.Get)
		v1.POST("/tracks", tracks.Create)
		v1.GET("/tracks/:id/stream", tracks.Stream)

		v1.GET("/playlists/:id", playlists.Get)
		v1.POST("/playlists", playlists.Create)
		v1.GET("/playlists/:id/tracks", playlists.ListTracks)
		v1.POST("/playlists/:id/tracks", playlists.AddTrack)
		v1.DELETE("/playlists/:id/tracks/:track_id", playlists.RemoveTrack)

		v1.GET("/search", search.Search)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("beatstream API listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	<-quit
	log.Println("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("server shutdown: %v", err)
	}
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required env var %s is not set", key)
	}
	return v
}
