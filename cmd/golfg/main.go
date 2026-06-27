// Command golfg is the entrypoint. It loads config, opens the SQLite store,
// builds the Fiber server and runs it until a termination signal arrives,
// then shuts down gracefully. Built to run as a foreground process under a
// container/process manager (Docker, LXD, systemd).
package main

import (
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/XotoX1337/golfg/internal/config"
	"github.com/XotoX1337/golfg/internal/server"
	"github.com/XotoX1337/golfg/internal/store"
	"github.com/spf13/pflag"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	portFlag := pflag.IntP("port", "p", 0, "Port to listen on (overrides config)")
	pflag.Parse()

	dataDir := executableDir()

	logger, err := buildLogger(filepath.Join(dataDir, config.ServiceName+".log"))
	if err != nil {
		log.Fatalf("logger: %v", err)
	}
	defer logger.Sync() //nolint:errcheck

	cfg, err := config.Load(dataDir, logger)
	if err != nil {
		logger.Fatal("load config", zap.Error(err))
	}
	if *portFlag != 0 {
		cfg.App.Port = *portFlag
	}

	st, err := store.Open(cfg.DBFile, logger)
	if err != nil {
		logger.Fatal("open store", zap.Error(err))
	}
	defer st.Close()

	srv, err := server.New(cfg, st, logger)
	if err != nil {
		logger.Fatal("build server", zap.Error(err))
	}

	// Run the server in the background and wait for a termination signal so we
	// can shut down gracefully (drain in-flight requests, close the store).
	go func() {
		if err := srv.Listen(); err != nil {
			logger.Error("server stopped", zap.Error(err))
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	sig := <-stop
	logger.Info("shutting down", zap.String("signal", sig.String()))

	if err := srv.Shutdown(); err != nil {
		logger.Error("shutdown error", zap.Error(err))
	}
}

// executableDir is where config, log and database files live (next to the
// binary). Falls back to the current directory if the path can't be resolved.
func executableDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

// buildLogger writes JSON logs to both stdout and the given file.
func buildLogger(logPath string) (*zap.Logger, error) {
	cfg := zap.NewProductionConfig()
	cfg.EncoderConfig.TimeKey = "timestamp"
	cfg.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
	cfg.OutputPaths = []string{"stdout", logPath}
	return cfg.Build()
}
