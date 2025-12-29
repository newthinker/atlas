package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New creates a new zap logger
func New(development bool) (*zap.Logger, error) {
	var cfg zap.Config

	if development {
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		cfg = zap.NewProductionConfig()
	}

	return cfg.Build()
}

// Must creates a logger or panics
func Must(development bool) *zap.Logger {
	log, err := New(development)
	if err != nil {
		panic(err)
	}
	return log
}
