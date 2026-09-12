package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/habiohq/habio"
	"github.com/habiohq/habio/adapter/httpapi"
	storesqlite "github.com/habiohq/habio/eventlog/sqlite"
	"github.com/habiohq/habio/execution"
	"github.com/habiohq/habio/provider/homeassistant"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	config, err := loadConfig()
	if err != nil {
		return err
	}
	provider, err := homeassistant.New(homeassistant.Config{
		BaseURL: config.homeAssistantURL, Token: config.homeAssistantToken, Bindings: config.bindings,
		Client: &http.Client{Timeout: config.providerTimeout},
	})
	if err != nil {
		return err
	}
	journal, err := storesqlite.Open(config.databasePath)
	if err != nil {
		return err
	}
	defer journal.Close()
	verifier, err := homeassistant.NewStateVerifier(config.verificationMaxAge)
	if err != nil {
		return err
	}
	execute, err := execution.NewExecuteAction(execution.ExecuteConfig{
		Admitter: allowAll{}, Resolver: provider, Provider: provider, Journal: journal,
	})
	if err != nil {
		return err
	}
	verify, err := execution.NewVerifyAttempt(execution.VerifyConfig{
		Resolver: provider, Observer: provider, Verifier: verifier, Journal: journal, Reader: journal, Actions: journal,
	})
	if err != nil {
		return err
	}
	recoverUseCase, err := execution.NewRecoverAttempt(execute, journal)
	if err != nil {
		return err
	}
	getAttempt, _ := execution.NewGetAttempt(journal)
	scanner, _ := execution.NewScanIncompleteAttempts(journal, journal)
	handler, err := httpapi.New(httpapi.Config{
		Executor: execute, Verifier: verify, Recoverer: recoverUseCase,
		GetAttempt: getAttempt, Scanner: scanner, APIToken: config.apiToken,
	})
	if err != nil {
		return err
	}

	server := &http.Server{
		Addr: config.listenAddress, Handler: handler,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serverErr := make(chan error, 1)
	go func() { serverErr <- server.ListenAndServe() }()
	log.Printf("Habio listening on %s", config.listenAddress)
	select {
	case err := <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}

type config struct {
	listenAddress      string
	databasePath       string
	apiToken           string
	homeAssistantURL   string
	homeAssistantToken string
	bindings           map[string]string
	providerTimeout    time.Duration
	verificationMaxAge time.Duration
}

func loadConfig() (config, error) {
	result := config{
		listenAddress: envOr("HABIO_LISTEN", "127.0.0.1:8080"),
		databasePath:  envOr("HABIO_DATABASE", "habio.db"), apiToken: os.Getenv("HABIO_API_TOKEN"),
		homeAssistantURL:   os.Getenv("HABIO_HOME_ASSISTANT_URL"),
		homeAssistantToken: os.Getenv("HABIO_HOME_ASSISTANT_TOKEN"),
		providerTimeout:    10 * time.Second, verificationMaxAge: 30 * time.Second,
	}
	if result.homeAssistantURL == "" || result.homeAssistantToken == "" {
		return config{}, errors.New("HABIO_HOME_ASSISTANT_URL and HABIO_HOME_ASSISTANT_TOKEN are required")
	}
	if err := json.Unmarshal([]byte(os.Getenv("HABIO_HOME_ASSISTANT_BINDINGS")), &result.bindings); err != nil || len(result.bindings) == 0 {
		return config{}, fmt.Errorf("HABIO_HOME_ASSISTANT_BINDINGS must be a non-empty JSON object: %w", err)
	}
	if value := os.Getenv("HABIO_PROVIDER_TIMEOUT"); value != "" {
		duration, err := time.ParseDuration(value)
		if err != nil || duration <= 0 {
			return config{}, errors.New("HABIO_PROVIDER_TIMEOUT must be a positive duration")
		}
		result.providerTimeout = duration
	}
	if value := os.Getenv("HABIO_VERIFICATION_MAX_AGE"); value != "" {
		duration, err := time.ParseDuration(value)
		if err != nil || duration <= 0 {
			return config{}, errors.New("HABIO_VERIFICATION_MAX_AGE must be a positive duration")
		}
		result.verificationMaxAge = duration
	}
	if result.apiToken == "" && !loopbackAddress(result.listenAddress) {
		return config{}, errors.New("HABIO_API_TOKEN is required when HABIO_LISTEN is not loopback")
	}
	return result, nil
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func loopbackAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

type allowAll struct{}

func (allowAll) Admit(context.Context, habio.Action) (habio.AdmissionStatus, error) {
	return habio.AdmissionAdmitted, nil
}

var _ habio.Admitter = allowAll{}
