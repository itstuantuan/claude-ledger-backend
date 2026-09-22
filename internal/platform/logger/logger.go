package logger

import "go.uber.org/zap"

type Logger = zap.Logger
type Field = zap.Field

var Error = zap.Error
var String = zap.String
var Int = zap.Int
var Duration = zap.Duration

func New(environment, level string) (*zap.Logger, error) {
	cfg := zap.NewProductionConfig()
	if environment == "development" || environment == "test" {
		cfg = zap.NewDevelopmentConfig()
	}
	if err := cfg.Level.UnmarshalText([]byte(level)); err != nil {
		return nil, err
	}
	return cfg.Build()
}
