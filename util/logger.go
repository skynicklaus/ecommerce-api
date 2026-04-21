package util

import (
	"log/slog"
	"os"

	"github.com/go-chi/httplog/v3"
)

type ServerLogger struct {
	*slog.Logger

	LogFormat *httplog.Schema
}

func NewLogger() *ServerLogger {
	var level slog.Level
	environment := os.Getenv("APP_ENV")
	appName := os.Getenv("APP_NAME")
	appVersion := os.Getenv("APP_VERSION")

	switch environment {
	case "production":
		level = slog.LevelWarn
	case "staging":
		level = slog.LevelInfo
	case "local", "development":
		level = slog.LevelDebug
	default:
		level = slog.LevelDebug
	}

	isLocal := environment == "local" || environment == "development" || environment == ""

	logFormat := httplog.SchemaECS.Concise(isLocal)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:       level,
		AddSource:   !isLocal,
		ReplaceAttr: logFormat.ReplaceAttr,
	}))

	if !isLocal {
		logger = logger.With(
			slog.String("app", appName),
			slog.String("version", appVersion),
			slog.String("env", environment),
		)
	}

	return &ServerLogger{
		Logger:    logger,
		LogFormat: logFormat,
	}
}

func (sl *ServerLogger) Fatal(msg string, args ...any) {
	sl.Error(msg, args...)
	os.Exit(1)
}
