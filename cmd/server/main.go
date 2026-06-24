// Command server runs the gateway, the admin API, and the embedded admin UI in
// a single process on a single port:
//
//	/admin/api/*  -> admin REST API
//	/admin, /admin/* -> embedded React admin UI
//	everything else  -> gateway data plane (user-configured routes)
//
// This is the all-in-one deployment. For a split deployment (separate ports /
// nginx) use cmd/gateway and cmd/admin instead.
package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"light-api-gateway/internal/adminapi"
	"light-api-gateway/internal/auth"
	"light-api-gateway/internal/config"
	"light-api-gateway/internal/gateway"
	"light-api-gateway/internal/health"
	"light-api-gateway/internal/store"
	"light-api-gateway/internal/webui"
)

func main() {
	configPath := flag.String("config", "configs/config.example.json", "gateway config (settings + route seed)")
	dbPath := flag.String("db", "data/gateway.db", "sqlite database path")
	listenOverride := flag.String("listen", "", "override listen address, for example :8080")
	seedConfigPath := flag.String("seed-config", "", "json config to seed routes when the table is empty (defaults to -config)")
	reloadInterval := flag.Duration("reload-interval", 2*time.Second, "sqlite config reload interval")
	healthCheckInterval := flag.Duration("health-check-interval", 10*time.Second, "upstream health check interval; set to 0 to disable")
	healthCheckTimeout := flag.Duration("health-check-timeout", 2*time.Second, "upstream health check request timeout")
	healthAutoDisableThreshold := flag.Int("health-auto-disable-threshold", 3, "consecutive failed health checks before auto-disabling an upstream target; set to 0 to disable")
	bootstrapUsername := flag.String("bootstrap-username", envOrDefault("GATEWAY_ADMIN_USERNAME", "admin"), "bootstrap admin username")
	bootstrapPassword := flag.String("bootstrap-password", envOrDefault("GATEWAY_ADMIN_PASSWORD", "admin123456"), "bootstrap admin password")
	tokenSecret := flag.String("token-secret", os.Getenv("GATEWAY_ADMIN_SECRET"), "admin api token secret")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx := context.Background()

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("load config failed", "error", err)
		os.Exit(1)
	}
	if *listenOverride != "" {
		cfg.Gateway.Listen = *listenOverride
	}

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

	seedPath := *seedConfigPath
	if seedPath == "" {
		seedPath = *configPath
	}
	if err := seedRoutesIfNeeded(ctx, st, seedPath, logger); err != nil {
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

	runtimeCfg, err := st.LoadConfig(ctx, cfg.Gateway)
	if err != nil {
		logger.Error("load sqlite config failed", "error", err)
		os.Exit(1)
	}

	gw, err := gateway.NewWithRequestLogStore(runtimeCfg, logger, st)
	if err != nil {
		logger.Error("create gateway failed", "error", err)
		os.Exit(1)
	}

	rootCtx, stopWorkers := context.WithCancel(context.Background())
	defer stopWorkers()
	go reloadSQLiteConfig(rootCtx, st, gw, cfg.Gateway, *reloadInterval, logger)
	if *healthCheckInterval > 0 {
		checker := health.NewChecker(
			st,
			logger,
			health.WithInterval(*healthCheckInterval),
			health.WithClient(&http.Client{Timeout: *healthCheckTimeout}),
			health.WithAutoDisableThreshold(*healthAutoDisableThreshold),
		)
		go checker.Run(rootCtx)
	}

	tokenManager, err := auth.NewTokenManager(*tokenSecret, 12*time.Hour)
	if err != nil {
		logger.Error("create token manager failed", "error", err)
		os.Exit(1)
	}

	handler := &rootHandler{
		admin:   adminapi.New(st, tokenManager, logger),
		ui:      webui.Handler(),
		gateway: gw,
	}

	server := &http.Server{
		Addr:         cfg.Gateway.ListenOrDefault(),
		Handler:      handler,
		ReadTimeout:  secondsOrDefault(cfg.Gateway.ReadTimeoutSeconds, 10),
		WriteTimeout: secondsOrDefault(cfg.Gateway.WriteTimeoutSeconds, 30),
		IdleTimeout:  secondsOrDefault(cfg.Gateway.IdleTimeoutSeconds, 60),
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("all-in-one server started",
			"listen", server.Addr,
			"admin_ui", server.Addr+webui.Mount,
			"admin_api", server.Addr+"/admin/api")
		errCh <- server.ListenAndServe()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			logger.Error("server stopped unexpectedly", "error", err)
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
		logger.Info("server stopped")
	}
}

// rootHandler dispatches a single port between the admin API, the admin UI, and
// the gateway data plane by path prefix. The /admin prefix is reserved for the
// admin plane; all other paths are handled by the gateway's configured routes.
type rootHandler struct {
	admin   http.Handler
	ui      http.Handler
	gateway http.Handler
}

func (h *rootHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case path == "/admin/api" || strings.HasPrefix(path, "/admin/api/"):
		h.admin.ServeHTTP(w, r)
	case path == webui.Mount || strings.HasPrefix(path, webui.Mount+"/"):
		h.ui.ServeHTTP(w, r)
	default:
		h.gateway.ServeHTTP(w, r)
	}
}

func reloadSQLiteConfig(ctx context.Context, st *store.Store, handler *gateway.Gateway, gatewayCfg config.GatewayConfig, interval time.Duration, logger *slog.Logger) {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cfg, err := st.LoadConfig(ctx, gatewayCfg)
			if err != nil {
				logger.Warn("reload sqlite config failed", "error", err)
				continue
			}
			if err := handler.Reload(cfg); err != nil {
				logger.Warn("apply sqlite config failed", "error", err)
			}
		}
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

func secondsOrDefault(value int, fallback int) time.Duration {
	if value <= 0 {
		value = fallback
	}
	return time.Duration(value) * time.Second
}
