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
	if config.listenAddress != "127.0.0.1:8080" || config.databasePath != "atoha.db" {
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
	t.Setenv("ATOHA_LISTEN", "0.0.0.0:8080")
	if _, err := loadConfig(); err == nil {
		t.Fatal("loadConfig() accepted an unauthenticated non-loopback listener")
	}
	t.Setenv("ATOHA_API_TOKEN", "api-secret")
	if _, err := loadConfig(); err != nil {
		t.Fatalf("loadConfig() with API token error = %v", err)
	}
}

func TestLoadConfigRejectsInvalidDuration(t *testing.T) {
	setRequiredEnvironment(t)
	t.Setenv("ATOHA_PROVIDER_TIMEOUT", "never")
	if _, err := loadConfig(); err == nil {
		t.Fatal("loadConfig() accepted an invalid timeout")
	}
}

func setRequiredEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"ATOHA_LISTEN", "ATOHA_DATABASE", "ATOHA_API_TOKEN", "ATOHA_PROVIDER_TIMEOUT", "ATOHA_VERIFICATION_MAX_AGE",
	} {
		t.Setenv(name, "")
	}
	t.Setenv("ATOHA_HOME_ASSISTANT_URL", "http://home-assistant.local:8123")
	t.Setenv("ATOHA_HOME_ASSISTANT_TOKEN", "ha-secret")
	t.Setenv("ATOHA_HOME_ASSISTANT_BINDINGS", `{"light":"light.living_room"}`)
}
