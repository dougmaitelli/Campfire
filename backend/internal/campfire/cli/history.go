package cli

import (
	"fmt"
	"strconv"

	"campfire/internal/campfire/service"
)

func parseHistoryRetention(audit, operations string) (service.HistoryRetention, error) {
	auditDays, err := strconv.Atoi(audit)
	if err != nil {
		return service.HistoryRetention{}, fmt.Errorf("audit retention days must be an integer: %w", err)
	}

	operationDays, err := strconv.Atoi(operations)
	if err != nil {
		return service.HistoryRetention{}, fmt.Errorf("operation retention days must be an integer: %w", err)
	}

	retention := service.HistoryRetention{AuditDays: auditDays, OperationDays: operationDays}

	return retention, retention.Validate()
}
