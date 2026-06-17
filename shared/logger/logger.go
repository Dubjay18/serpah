package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New initializes a production-ready Zap logger.
func New(service string) *zap.Logger {
	config := zap.NewProductionConfig()
	
	// Customize encoder config to use standard readable time formatting
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	
	// Read log level from environment variable (default is Info)
	levelStr := os.Getenv("LOG_LEVEL")
	if levelStr != "" {
		var level zapcore.Level
		if err := level.UnmarshalText([]byte(levelStr)); err == nil {
			config.Level = zap.NewAtomicLevelAt(level)
		}
	}

	zapLogger, err := config.Build()
	if err != nil {
		// Fallback to a basic logger if construction fails
		zapLogger = zap.NewExample()
	}

	return zapLogger.With(zap.String("service", service))
}
