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

	"light-api-gateway/internal/config"
	"light-api-gateway/internal/gateway"
	"light-api-gateway/internal/health"
	"light-api-gateway/internal/store"
)

func main() {
	configPath := flag.String("config", "configs/config.example.json", "path to gateway config file")
	dbPath := flag.String("db", "", "sqlite database path; when set, routes are loaded from sqlite")
	listenOverride := flag.String("listen", "", "override listen address, for example :8080")
	reloadInterval := flag.Duration("reload-interval", 2*time.Second, "sqlite config reload interval")
	healthCheckInterval := flag.Duration("health-check-interval", 10*time.Second, "sqlite upstream health check interval; set to 0 to disable")
	healthCheckTimeout := flag.Duration("health-check-timeout", 2*time.Second, "upstream health check request timeout")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("load config failed", "error", err)
		os.Exit(1)
	}
	if *listenOverride != "" {
		cfg.Gateway.Listen = *listenOverride
	}

	var st *store.Store
	if *dbPath != "" {
		st, err = store.Open(*dbPath)
		if err != nil {
			logger.Error("open store failed", "error", err)
			os.Exit(1)
		}
		defer st.Close()
		if err := st.Migrate(context.Background()); err != nil {
			logger.Error("migrate store failed", "error", err)
			os.Exit(1)
		}
		count, err := st.CountRoutes(context.Background())
		if err != nil {
			logger.Error("count routes failed", "error", err)
			os.Exit(1)
		}
		if count == 0 && len(cfg.Routes) > 0 {
			if err := st.SeedConfig(context.Background(), cfg); err != nil {
				logger.Error("seed sqlite config failed", "error", err)
				os.Exit(1)
			}
			logger.Info("seeded sqlite config from json", "routes", len(cfg.Routes), "db", *dbPath)
		}
		cfg, err = st.LoadConfig(context.Background(), cfg.Gateway)
		if err != nil {
			logger.Error("load sqlite config failed", "error", err)
			os.Exit(1)
		}
	}

	handler, err := gateway.NewWithRequestLogStore(cfg, logger, st)
	if err != nil {
		logger.Error("create gateway failed", "error", err)
		os.Exit(1)
	}

	rootCtx, stopReload := context.WithCancel(context.Background())
	defer stopReload()
	if st != nil {
		go reloadSQLiteConfig(rootCtx, st, handler, cfg.Gateway, *reloadInterval, logger)
		if *healthCheckInterval > 0 {
			checker := health.NewChecker(
				st,
				logger,
				health.WithInterval(*healthCheckInterval),
				health.WithClient(&http.Client{Timeout: *healthCheckTimeout}),
			)
			go checker.Run(rootCtx)
		}
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
		logger.Info("gateway started", "listen", server.Addr, "config", *configPath)
		errCh <- server.ListenAndServe()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			logger.Error("gateway stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	case sig := <-stop:
		logger.Info("shutdown signal received", "signal", sig.String())
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			logger.Error("graceful shutdown failed", "error", err)
			os.Exit(1)
		}
		logger.Info("gateway stopped")
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

func secondsOrDefault(value int, fallback int) time.Duration {
	if value <= 0 {
		value = fallback
	}
	return time.Duration(value) * time.Second
}
