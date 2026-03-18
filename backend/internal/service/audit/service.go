package audit

import (
	"context"
	"log/slog"

	auditrepo "github.com/kana-consultant/kantor/backend/internal/repository/audit"
)

type Entry = auditrepo.Entry

type Service struct {
	repo *auditrepo.Repository
}

func NewService(repo *auditrepo.Repository) *Service {
	return &Service{repo: repo}
}

// Log writes an audit entry. It never returns an error to the caller;
// failures are logged so that audit issues do not break business operations.
func (s *Service) Log(ctx context.Context, entry Entry) {
	if err := s.repo.Insert(ctx, entry); err != nil {
		slog.Error("failed to write audit log",
			"error", err,
			"action", entry.Action,
			"module", entry.Module,
			"resource", entry.Resource,
			"resource_id", entry.ResourceID,
			"user_id", entry.UserID,
		)
	}
}
