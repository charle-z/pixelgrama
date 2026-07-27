package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/charle-z/pixelgrama/internal/app"
	"github.com/charle-z/pixelgrama/internal/ratelimit"
	"github.com/charle-z/pixelgrama/internal/store"
)

var (
	Commit  = "development"
	RepoURL = "https://github.com/charle-z/pixelgrama"
	PRURL   = "https://github.com/charle-z/pixelgrama/pulls"
)

func main() {
	address := envOrDefault("ADDR", ":8080")
	databasePath := envOrDefault("DB_PATH", "/data/pixelgrama.db")
	trustProxy := envBool("TRUST_PROXY", false)

	if err := os.MkdirAll(filepath.Dir(databasePath), 0o750); err != nil {
		log.Fatalf("create data directory: %v", err)
	}
	database, err := store.Open(databasePath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer database.Close()

	handler, err := app.New(app.Config{
		Store:      database,
		Limiter:    ratelimit.New(5, time.Minute, 10000, time.Now),
		Commit:     Commit,
		RepoURL:    RepoURL,
		PRURL:      PRURL,
		Now:        time.Now,
		BodyLimit:  4096,
		TrustProxy: trustProxy,
	})
	if err != nil {
		log.Fatalf("configure server: %v", err)
	}

	server := &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}

	shutdownSignal, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-shutdownSignal.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("graceful shutdown: %v", err)
		}
	}()

	log.Printf("pixelgrama commit=%s listening=%s", Commit, address)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("serve: %v", err)
	}
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func envBool(name string, fallback bool) bool {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		log.Fatalf("%s must be a boolean", name)
	}
	return parsed
}
