package exportutil

import (
	"context"
	"strings"

	"github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/model"
)

type UserLookup interface {
	GetUserByID(ctx context.Context, userID string) (model.User, error)
}

func ResolveGeneratedBy(ctx context.Context, lookup UserLookup) string {
	principal, ok := middleware.PrincipalFromContext(ctx)
	if !ok {
		return "KANTOR"
	}

	if lookup != nil {
		user, err := lookup.GetUserByID(ctx, principal.UserID)
		if err == nil {
			name := strings.TrimSpace(user.FullName)
			if name != "" {
				return name
			}
		}
	}

	if strings.TrimSpace(principal.UserID) != "" {
		return principal.UserID
	}

	return "KANTOR"
}
