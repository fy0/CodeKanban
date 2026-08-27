package websession

import (
	"context"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

const slowWebSessionTransactionThreshold = 250 * time.Millisecond

func (m *Manager) observedTransaction(
	ctx context.Context,
	db *gorm.DB,
	operation string,
	transaction func(*gorm.DB) error,
	fields ...zap.Field,
) error {
	startedAt := time.Now()
	err := db.WithContext(ctx).Transaction(transaction)
	duration := time.Since(startedAt)
	if duration < slowWebSessionTransactionThreshold || m == nil || m.logger == nil {
		return err
	}

	logFields := []zap.Field{
		zap.String("operation", operation),
		zap.Duration("duration", duration),
		zap.Duration("threshold", slowWebSessionTransactionThreshold),
		zap.Bool("success", err == nil),
	}
	logFields = append(logFields, fields...)
	if err != nil {
		logFields = append(logFields, zap.Error(err))
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		logFields = append(logFields, zap.String("contextError", ctxErr.Error()))
	}
	m.logger.Warn("slow web session database transaction", logFields...)
	return err
}
