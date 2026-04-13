package main

import (
	"bufio"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"boot.dev/linko/internal/build"
	"boot.dev/linko/internal/linkoerr"
	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
	pkgerr "github.com/pkg/errors"
)

type closeFunc func() error

type stackTracer interface {
	error
	StackTrace() pkgerr.StackTrace
}

type muiltiError interface {
	error
	Unwrap() []error
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

	env := os.Getenv("ENV")
	hostname, _ := os.Hostname()

	logger := slog.New(slog.NewMultiHandler(handlers...))
	logger = logger.With(
		slog.String("git_sha", build.GitSHA),
		slog.String("build_time", build.BuildTime),
		slog.String("env", env),
		slog.String("hostname", hostname),
	)

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

func errAttrs(err error) []slog.Attr {
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

	return attrs
}

func replaceAttr(groups []string, a slog.Attr) slog.Attr {
	if a.Key == "error" {
		err, ok := a.Value.Any().(error)

		if !ok {
			return a
		}

		if muiltiErr, ok := errors.AsType[muiltiError](err); ok {
			var multiErrAttr []slog.Attr

			for i, me := range muiltiErr.Unwrap() {
				attrs := errAttrs(me)
				multiErrAttr = append(multiErrAttr, slog.GroupAttrs(
					fmt.Sprintf("error_%d", i+1),
					attrs...,
				))
			}

			return slog.GroupAttrs(
				"errors",
				multiErrAttr...,
			)
		}

		return slog.GroupAttrs(
			"error",
			errAttrs(err)...,
		)
	}

	return a
}

func getStdLogHandler() (slog.Handler, closeFunc, error) {
	opts := tint.Options{
		Level:       slog.LevelDebug,
		ReplaceAttr: replaceAttr,
		NoColor:     true,
	}

	if isatty.IsCygwinTerminal(os.Stderr.Fd()) || isatty.IsTerminal(os.Stderr.Fd()) {
		opts.NoColor = false
	}

	handler := tint.NewHandler(os.Stderr, &opts)

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
