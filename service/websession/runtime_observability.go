package websession

import (
	"time"

	"go.uber.org/zap"
)

const slowRuntimeCapabilityProbeThreshold = time.Second

func (m *Manager) logRuntimeCapabilityProbe(
	probe string,
	startedAt time.Time,
	err error,
	fields ...zap.Field,
) {
	if m == nil || m.logger == nil {
		return
	}
	duration := time.Since(startedAt)
	result := "success"
	errorCode := ""
	if err != nil {
		result = "failed"
		errorCode = "probe_failed"
	}
	baseFields := []zap.Field{
		zap.String("probe", probe),
		zap.String("result", result),
		zap.String("errorCode", errorCode),
		zap.Duration("duration", duration),
	}
	baseFields = append(baseFields, fields...)
	if err != nil || duration >= slowRuntimeCapabilityProbeThreshold {
		m.logger.Warn("web session runtime capability probe completed", baseFields...)
		return
	}
	m.logger.Info("web session runtime capability probe completed", baseFields...)
}
