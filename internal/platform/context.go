package platform

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

type contextKey string

const (
	CorrelationIDKey contextKey = "correlation_id"
	MerchantIDKey    contextKey = "merchant_id"
)

func WithCorrelationID(ctx context.Context, correlationID string) context.Context {
	if correlationID == "" {
		correlationID = uuid.New().String()
	}
	return context.WithValue(ctx, CorrelationIDKey, correlationID)
}

func GetCorrelationID(ctx context.Context) string {
	if correlationID, ok := ctx.Value(CorrelationIDKey).(string); ok {
		return correlationID
	}
	return ""
}

func WithMerchantID(ctx context.Context, merchantID string) context.Context {
	return context.WithValue(ctx, MerchantIDKey, merchantID)
}

func GetMerchantID(ctx context.Context) string {
	if merchantID, ok := ctx.Value(MerchantIDKey).(string); ok {
		return merchantID
	}
	return ""
}

func GenerateID(prefix string) string {
	id := uuid.New().String()
	id = strings.ReplaceAll(id, "-", "")
	if prefix != "" {
		return prefix + "_" + id
	}
	return id
}
