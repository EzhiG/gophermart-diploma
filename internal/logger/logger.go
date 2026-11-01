// Package logger создает логгер middleware для логирования HTTP-запросов.
package logger

import "go.uber.org/zap"

// New создает логгер в prod-режиме.
func New() (*zap.SugaredLogger, error) {
	logger, err := zap.NewProduction()
	if err != nil {
		return nil, err
	}

	return logger.Sugar(), nil
}
