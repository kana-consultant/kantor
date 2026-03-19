package marketing

import (
	"context"
	"strings"

	notificationsrepo "github.com/kana-consultant/kantor/backend/internal/repository/notifications"
)

type notificationSender interface {
	CreateMany(ctx context.Context, params []notificationsrepo.CreateParams) error
}

func sendNotifications(
	ctx context.Context,
	service notificationSender,
	userIDs []string,
	notificationType string,
	title string,
	message string,
	referenceType string,
	referenceID *string,
) error {
	if service == nil {
		return nil
	}

	params := make([]notificationsrepo.CreateParams, 0, len(userIDs))
	for _, userID := range uniqueNotificationUserIDs(userIDs) {
		if strings.TrimSpace(userID) == "" {
			continue
		}

		refType := referenceType
		params = append(params, notificationsrepo.CreateParams{
			UserID:        userID,
			Type:          notificationType,
			Title:         title,
			Message:       message,
			ReferenceType: &refType,
			ReferenceID:   referenceID,
		})
	}

	if len(params) == 0 {
		return nil
	}

	return service.CreateMany(ctx, params)
}

func uniqueNotificationUserIDs(userIDs []string) []string {
	seen := map[string]struct{}{}
	items := make([]string, 0, len(userIDs))
	for _, userID := range userIDs {
		trimmed := strings.TrimSpace(userID)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		items = append(items, trimmed)
	}
	return items
}
