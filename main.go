package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"boot.dev/linko/internal/store"
)

type closeFunc func() error

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	httpPort := flag.Int("port", 8899, "port to listen on")
	dataDir := flag.String("data", "./data", "directory to store data")
	flag.Parse()

	status := run(ctx, cancel, *httpPort, *dataDir)
	cancel()
	os.Exit(status)
}

func initializeLogger() (*log.Logger, closeFunc, error) {
	filepath := os.Getenv("LINKO_LOG_FILE")

	if filepath != "" {
		file, err := os.OpenFile(filepath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
		bufferedFile := bufio.NewWriterSize(file, 8192)

		if err != nil {
			return nil, nil, fmt.Errorf("failed to open log file: %w", err)
		}

		multiWriter := io.MultiWriter(os.Stderr, bufferedFile)
		logger := log.New(multiWriter, "", log.LstdFlags)
		closeF := func() error {
			if err := bufferedFile.Flush(); err != nil {
				return fmt.Errorf("failed to flush log file: %w", err)
			}

			if err := file.Close(); err != nil {
				return fmt.Errorf("failed to close log file: %w", err)
			}

			return nil
		}
		return logger, closeF, nil
	}

	closeF := func() error {
		return nil
	}

	logger := log.New(os.Stderr, "", log.LstdFlags)

	return logger, closeF, nil
}

func run(ctx context.Context, cancel context.CancelFunc, httpPort int, dataDir string) int {
	logger, closeF, err := initializeLogger()

	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %s\n", err)
		return 1
	}

	defer func() {
		err := closeF()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to close logger: %v\n", err)
		}
	}()

	st, err := store.New(dataDir, logger)
	if err != nil {
		logger.Printf("failed to create store: %v", err)
		return 1
	}

	s := newServer(*st, httpPort, logger, cancel)
	var serverErr error
	go func() {
		serverErr = s.start()
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	logger.Println("Linko is shutting down")
	if err := s.shutdown(shutdownCtx); err != nil {
		logger.Printf("failed to shutdown server: %v", err)
		return 1
	}
	if serverErr != nil {
		logger.Printf("server error: %v", serverErr)
		return 1
	}
	return 0
}
