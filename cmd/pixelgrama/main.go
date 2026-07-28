package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
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
	config, err := loadConfig()
	if err != nil {
		log.Fatalf("configure runtime: %v", err)
	}
	if err := prepareRuntime(config.databasePath, systemRuntimeOps{}); err != nil {
		log.Fatalf("prepare runtime: %v", err)
	}
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "backup":
			if err := runBackup(context.Background(), config, os.Args[2:], os.Stdout); err != nil {
				log.Fatalf("backup: %v", err)
			}
		case "admin":
			if err := runAdmin(context.Background(), config, os.Args[2:], os.Stdout, os.Stderr, time.Now); err != nil {
				log.Fatalf("admin: %v", err)
			}
		default:
			log.Fatalf("unknown command %q", os.Args[1])
		}
		return
	}

	database, err := store.Open(config.databasePath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer database.Close()

	handler, err := app.New(app.Config{
		Store:             database,
		Limiter:           ratelimit.New(config.rateLimitRequests, config.rateLimitWindow, config.rateLimitMaxEntries, time.Now),
		Commit:            Commit,
		RepoURL:           RepoURL,
		PRURL:             PRURL,
		Now:               time.Now,
		BodyLimit:         4096,
		TrustedProxyCIDRs: config.trustedProxyCIDRs,
		RateLimitWindow:   config.rateLimitWindow,
	})
	if err != nil {
		log.Fatalf("configure server: %v", err)
	}

	server := &http.Server{
		Addr:              config.address,
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

	log.Printf("pixelgrama commit=%s listening=%s", Commit, config.address)
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
