package main

import (
	"testing"
	"time"
)

func TestLoadConfigDefaultsToAuthenticatedLocalRuntime(t *testing.T) {
	setRequiredEnvironment(t)
	config, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.listenAddress != "127.0.0.1:8080" || config.databasePath != "habio.db" {
		t.Fatalf("defaults = %q/%q", config.listenAddress, config.databasePath)
	}
	if config.providerTimeout != 10*time.Second || config.verificationMaxAge != 30*time.Second {
		t.Fatalf("durations = %s/%s", config.providerTimeout, config.verificationMaxAge)
	}
	if config.bindings["light"] != "light.living_room" {
		t.Fatalf("bindings = %#v", config.bindings)
	}
}

func TestLoadConfigRequiresTokenOffLoopback(t *testing.T) {
	setRequiredEnvironment(t)
	t.Setenv("HABIO_LISTEN", "0.0.0.0:8080")
	if _, err := loadConfig(); err == nil {
		t.Fatal("loadConfig() accepted an unauthenticated non-loopback listener")
	}
	t.Setenv("HABIO_API_TOKEN", "api-secret")
	if _, err := loadConfig(); err != nil {
		t.Fatalf("loadConfig() with API token error = %v", err)
	}
}

func TestLoadConfigRejectsInvalidDuration(t *testing.T) {
	setRequiredEnvironment(t)
	t.Setenv("HABIO_PROVIDER_TIMEOUT", "never")
	if _, err := loadConfig(); err == nil {
		t.Fatal("loadConfig() accepted an invalid timeout")
	}
}

func setRequiredEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"HABIO_LISTEN", "HABIO_DATABASE", "HABIO_API_TOKEN", "HABIO_PROVIDER_TIMEOUT", "HABIO_VERIFICATION_MAX_AGE",
	} {
		t.Setenv(name, "")
	}
	t.Setenv("HABIO_HOME_ASSISTANT_URL", "http://home-assistant.local:8123")
	t.Setenv("HABIO_HOME_ASSISTANT_TOKEN", "ha-secret")
	t.Setenv("HABIO_HOME_ASSISTANT_BINDINGS", `{"light":"light.living_room"}`)
}
