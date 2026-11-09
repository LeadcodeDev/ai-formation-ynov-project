package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ynov/equipment-loans/internal/adapters/auth"
	httpapi "github.com/ynov/equipment-loans/internal/adapters/http"
	mcpserver "github.com/ynov/equipment-loans/internal/adapters/mcp"
	"github.com/ynov/equipment-loans/internal/adapters/postgres"
	"github.com/ynov/equipment-loans/internal/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("api stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.NewPool(ctx, cfg.databaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()
	if err := postgres.Migrate(ctx, pool); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	verifier, err := auth.NewVerifier(ctx, cfg.auth)
	if err != nil {
		return fmt.Errorf("create verifier: %w", err)
	}
	svc := service.New(postgres.NewTransactor(pool), systemClock{}, uuidGenerator{})

	servers := []*http.Server{newServer(cfg.port, httpapi.NewHandler(svc, verifier, logger), restWriteTimeout)}
	if cfg.mcpPort != "" {
		servers = append(servers, newServer(cfg.mcpPort, mcpserver.NewHandler(svc, verifier, logger), 0))
	}
	return serve(ctx, logger, servers)
}

const (
	restWriteTimeout = 30 * time.Second
	readTimeout      = 30 * time.Second
	idleTimeout      = 2 * time.Minute
)

func newServer(port string, handler http.Handler, writeTimeout time.Duration) *http.Server {
	return &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}
}

func serve(ctx context.Context, logger *slog.Logger, servers []*http.Server) error {
	failures := make(chan error, len(servers))
	for _, srv := range servers {
		go func() {
			logger.Info("listening", "addr", srv.Addr)
			if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
				failures <- err
			}
		}()
	}
	select {
	case err := <-failures:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var errs []error
	for _, srv := range servers {
		errs = append(errs, srv.Shutdown(shutdownCtx))
	}
	return errors.Join(errs...)
}
