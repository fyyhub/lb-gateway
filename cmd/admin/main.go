package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"light-api-gateway/internal/adminapi"
	"light-api-gateway/internal/auth"
	"light-api-gateway/internal/config"
	"light-api-gateway/internal/store"
)

func main() {
	listen := flag.String("listen", ":8082", "admin api listen address")
	dbPath := flag.String("db", "data/gateway.db", "sqlite database path")
	seedConfigPath := flag.String("seed-config", "configs/config.example.json", "json config to seed when routes table is empty")
	bootstrapUsername := flag.String("bootstrap-username", envOrDefault("GATEWAY_ADMIN_USERNAME", "admin"), "bootstrap admin username")
	bootstrapPassword := flag.String("bootstrap-password", envOrDefault("GATEWAY_ADMIN_PASSWORD", "admin123456"), "bootstrap admin password")
	tokenSecret := flag.String("token-secret", os.Getenv("GATEWAY_ADMIN_SECRET"), "admin api token secret")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx := context.Background()

	st, err := store.Open(*dbPath)
	if err != nil {
		logger.Error("open store failed", "error", err)
		os.Exit(1)
	}
	defer st.Close()

	if err := st.Migrate(ctx); err != nil {
		logger.Error("migrate store failed", "error", err)
		os.Exit(1)
	}

	if err := seedRoutesIfNeeded(ctx, st, *seedConfigPath, logger); err != nil {
		logger.Error("seed routes failed", "error", err)
		os.Exit(1)
	}

	if err := ensureBootstrapAdmin(ctx, st, *bootstrapUsername, *bootstrapPassword, logger); err != nil {
		logger.Error("ensure bootstrap admin failed", "error", err)
		os.Exit(1)
	}
	if *tokenSecret == "" {
		logger.Warn("admin token secret is generated for this process; set GATEWAY_ADMIN_SECRET for stable tokens")
	}

	tokenManager, err := auth.NewTokenManager(*tokenSecret, 12*time.Hour)
	if err != nil {
		logger.Error("create token manager failed", "error", err)
		os.Exit(1)
	}

	server := &http.Server{
		Addr:              *listen,
		Handler:           adminapi.New(st, tokenManager, logger),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("admin api started", "listen", server.Addr, "db", *dbPath)
		errCh <- server.ListenAndServe()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			logger.Error("admin api stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	case sig := <-stop:
		logger.Info("shutdown signal received", "signal", sig.String())
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("graceful shutdown failed", "error", err)
			os.Exit(1)
		}
		logger.Info("admin api stopped")
	}
}

func seedRoutesIfNeeded(ctx context.Context, st *store.Store, seedConfigPath string, logger *slog.Logger) error {
	if seedConfigPath == "" {
		return nil
	}
	count, err := st.CountRoutes(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	cfg, err := config.Load(seedConfigPath)
	if err != nil {
		return err
	}
	if len(cfg.Routes) == 0 {
		return nil
	}

	if err := st.SeedConfig(ctx, cfg); err != nil {
		return err
	}
	logger.Info("seeded routes from config", "path", seedConfigPath, "routes", len(cfg.Routes))
	return nil
}

func ensureBootstrapAdmin(ctx context.Context, st *store.Store, username string, password string, logger *slog.Logger) error {
	count, err := st.CountAdminUsers(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	user, created, err := st.EnsureAdminUser(ctx, username, password)
	if err != nil {
		return err
	}
	if created {
		logger.Info("created bootstrap admin user", "username", user.Username)
	}
	return nil
}

func envOrDefault(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
