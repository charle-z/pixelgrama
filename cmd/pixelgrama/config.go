package main

import (
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"
)

type runtimeConfig struct {
	address             string
	databasePath        string
	trustedProxyCIDRs   []netip.Prefix
	rateLimitRequests   int
	rateLimitWindow     time.Duration
	rateLimitMaxEntries int
}

func loadConfig() (runtimeConfig, error) {
	config := runtimeConfig{
		address:             envOrDefault("ADDR", ":8080"),
		databasePath:        envOrDefault("DB_PATH", "/data/pixelgrama.db"),
		rateLimitRequests:   5,
		rateLimitWindow:     time.Minute,
		rateLimitMaxEntries: 10000,
	}
	var err error
	if config.trustedProxyCIDRs, err = parseCIDRs(os.Getenv("TRUSTED_PROXY_CIDRS")); err != nil {
		return runtimeConfig{}, fmt.Errorf("TRUSTED_PROXY_CIDRS: %w", err)
	}
	if config.rateLimitRequests, err = positiveEnvInt("RATE_LIMIT_REQUESTS", config.rateLimitRequests); err != nil {
		return runtimeConfig{}, err
	}
	if value := os.Getenv("RATE_LIMIT_WINDOW"); value != "" {
		config.rateLimitWindow, err = time.ParseDuration(value)
		if err != nil || config.rateLimitWindow <= 0 {
			return runtimeConfig{}, errorsf("RATE_LIMIT_WINDOW must be a positive duration")
		}
	}
	if config.rateLimitMaxEntries, err = positiveEnvInt("RATE_LIMIT_MAX_ENTRIES", config.rateLimitMaxEntries); err != nil {
		return runtimeConfig{}, err
	}
	return config, nil
}

func parseCIDRs(value string) ([]netip.Prefix, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	prefixes := make([]netip.Prefix, 0, len(parts))
	for _, part := range parts {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(part))
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q", strings.TrimSpace(part))
		}
		prefixes = append(prefixes, prefix.Masked())
	}
	return prefixes, nil
}

func positiveEnvInt(name string, fallback int) (int, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return parsed, nil
}

func errorsf(message string) error {
	return fmt.Errorf("%s", message)
}
