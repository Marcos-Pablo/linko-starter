package main

import (
	"bufio"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"boot.dev/linko/internal/linkoerr"
	pkgerr "github.com/pkg/errors"
)

type closeFunc func() error

type stackTracer interface {
	error
	StackTrace() pkgerr.StackTrace
}

func replaceAttr(groups []string, a slog.Attr) slog.Attr {
	if a.Key == "error" {
		err, ok := a.Value.Any().(error)

		if !ok {
			return a
		}

		attrs := []slog.Attr{
			slog.Attr{
				Key:   "message",
				Value: slog.StringValue(err.Error()),
			},
		}

		attrs = append(attrs, linkoerr.Attrs(err)...)

		if stackErr, ok := errors.AsType[stackTracer](err); ok {
			attrs = append(attrs, slog.Attr{
				Key:   "stack_trace",
				Value: slog.StringValue(fmt.Sprintf("%+v", stackErr.StackTrace())),
			})
		}

		return slog.GroupAttrs(
			"error",
			attrs...,
		)
	}

	return a
}

func initializeLogger() (*slog.Logger, closeFunc, error) {
	var handlers []slog.Handler
	var closers []closeFunc

	stdHandler, stdClose, err := getStdLogHandler()

	if err != nil {
		return nil, nil, fmt.Errorf("failed to create std log handler: %w", err)
	}

	handlers = append(handlers, stdHandler)
	closers = append(closers, stdClose)

	fileHandler, fileHandlClose, err := getFileLongHandler()

	if err != nil {
		return nil, nil, fmt.Errorf("failed to create log file handler: %w", err)
	}

	if fileHandler != nil {
		handlers = append(handlers, fileHandler)
		closers = append(closers, fileHandlClose)
	}

	logger := slog.New(slog.NewMultiHandler(handlers...))

	closeF := func() error {
		var errs []error
		for _, c := range closers {
			if err := c(); err != nil {
				errs = append(errs, err)
			}
		}

		return errors.Join(errs...)
	}

	return logger, closeF, nil
}

func getStdLogHandler() (slog.Handler, closeFunc, error) {
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level:       slog.LevelDebug,
		ReplaceAttr: replaceAttr,
	})

	closeF := func() error {
		return nil
	}

	return handler, closeF, nil
}

func getFileLongHandler() (slog.Handler, closeFunc, error) {
	filepath := os.Getenv("LINKO_LOG_FILE")

	if filepath == "" {
		return nil, nil, nil
	}

	file, err := os.OpenFile(filepath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	bufferedFile := bufio.NewWriterSize(file, 8192)

	if err != nil {
		return nil, nil, fmt.Errorf("failed to open log file: %w", err)
	}

	fileHandler := slog.NewJSONHandler(bufferedFile, &slog.HandlerOptions{
		Level:       slog.LevelInfo,
		ReplaceAttr: replaceAttr,
	})

	closeF := func() error {
		if err := bufferedFile.Flush(); err != nil {
			return fmt.Errorf("failed to flush log file: %w", err)
		}

		if err := file.Close(); err != nil {
			return fmt.Errorf("failed to close log file: %w", err)
		}

		return nil
	}
	return fileHandler, closeF, nil
}
