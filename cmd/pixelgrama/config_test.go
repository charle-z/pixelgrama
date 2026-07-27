package main

import (
	"net/netip"
	"testing"
	"time"
)

func TestLoadConfigUsesSafeDefaults(t *testing.T) {
	for _, name := range []string{
		"ADDR", "DB_PATH", "TRUSTED_PROXY_CIDRS",
		"RATE_LIMIT_REQUESTS", "RATE_LIMIT_WINDOW", "RATE_LIMIT_MAX_ENTRIES",
	} {
		t.Setenv(name, "")
	}
	config, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.address != ":8080" || config.databasePath != "/data/pixelgrama.db" {
		t.Fatalf("unexpected defaults: %#v", config)
	}
	if config.rateLimitRequests != 5 || config.rateLimitWindow != time.Minute || config.rateLimitMaxEntries != 10000 {
		t.Fatalf("unexpected rate defaults: %#v", config)
	}
	if len(config.trustedProxyCIDRs) != 0 {
		t.Fatalf("trusted CIDRs = %#v, want none", config.trustedProxyCIDRs)
	}
}

func TestLoadConfigParsesTrustedCIDRsAndRateLimit(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", "172.16.0.0/12, fc00::/7")
	t.Setenv("RATE_LIMIT_REQUESTS", "9")
	t.Setenv("RATE_LIMIT_WINDOW", "90s")
	t.Setenv("RATE_LIMIT_MAX_ENTRIES", "321")
	config, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	want := []netip.Prefix{netip.MustParsePrefix("172.16.0.0/12"), netip.MustParsePrefix("fc00::/7")}
	if len(config.trustedProxyCIDRs) != len(want) {
		t.Fatalf("CIDRs = %#v", config.trustedProxyCIDRs)
	}
	for i := range want {
		if config.trustedProxyCIDRs[i] != want[i] {
			t.Fatalf("CIDR %d = %v, want %v", i, config.trustedProxyCIDRs[i], want[i])
		}
	}
	if config.rateLimitRequests != 9 || config.rateLimitWindow != 90*time.Second || config.rateLimitMaxEntries != 321 {
		t.Fatalf("unexpected rate config: %#v", config)
	}
}

func TestLoadConfigRejectsInvalidOperationalValues(t *testing.T) {
	for _, test := range []struct {
		name  string
		key   string
		value string
	}{
		{name: "invalid CIDR", key: "TRUSTED_PROXY_CIDRS", value: "private-network"},
		{name: "zero requests", key: "RATE_LIMIT_REQUESTS", value: "0"},
		{name: "invalid duration", key: "RATE_LIMIT_WINDOW", value: "minute"},
		{name: "negative entries", key: "RATE_LIMIT_MAX_ENTRIES", value: "-1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, name := range []string{"TRUSTED_PROXY_CIDRS", "RATE_LIMIT_REQUESTS", "RATE_LIMIT_WINDOW", "RATE_LIMIT_MAX_ENTRIES"} {
				t.Setenv(name, "")
			}
			t.Setenv(test.key, test.value)
			if _, err := loadConfig(); err == nil {
				t.Fatal("expected configuration error")
			}
		})
	}
}
