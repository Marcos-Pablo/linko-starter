package main

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
)

type closeFunc func() error

func initializeLogger() (*slog.Logger, closeFunc, error) {
	replaceAttr := func(groups []string, a slog.Attr) slog.Attr {
		if a.Key == "error" {
			err, ok := a.Value.Any().(error)

			if !ok {
				return a
			}

			return slog.String("error", fmt.Sprintf("%+v", err))
		}

		return a
	}

	stdErrorHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level:       slog.LevelDebug,
		ReplaceAttr: replaceAttr,
	})

	filepath := os.Getenv("LINKO_LOG_FILE")
	if filepath != "" {
		file, err := os.OpenFile(filepath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
		bufferedFile := bufio.NewWriterSize(file, 8192)

		if err != nil {
			return nil, nil, fmt.Errorf("failed to open log file: %w", err)
		}

		fileHandler := slog.NewJSONHandler(bufferedFile, &slog.HandlerOptions{
			Level:       slog.LevelInfo,
			ReplaceAttr: replaceAttr,
		})

		combinedHandler := slog.NewMultiHandler(stdErrorHandler, fileHandler)

		logger := slog.New(combinedHandler)

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

	logger := slog.New(stdErrorHandler)

	return logger, closeF, nil
}
