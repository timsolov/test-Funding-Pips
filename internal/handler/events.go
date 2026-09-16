package handler

import (
	"context"
	"fmt"
	"time"
)

const opTimeout = 5 * time.Second

type Event struct {
	RequestID string  `json:"request_id"`
	Operation string  `json:"operation"`
	Status    string  `json:"status"`
	Reason    *string `json:"reason"`
	Timestamp string  `json:"timestamp"`
}

func publishCompleted(ctx context.Context, nc Publisher, requestID, operation string) {
	if err := nc.PublishEvent(ctx, "wallet.events.completed", Event{
		RequestID: requestID,
		Operation: operation,
		Status:    "completed",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		fmt.Println("publish completed failed:", err)
	}
}

func publishFailed(ctx context.Context, nc Publisher, requestID, operation, reason string) {
	if err := nc.PublishEvent(ctx, "wallet.events.failed", Event{
		RequestID: requestID,
		Operation: operation,
		Status:    "failed",
		Reason:    &reason,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		fmt.Println("publish failed event failed:", err)
	}
}
