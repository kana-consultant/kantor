package audit

import (
	"context"
	"encoding/csv"
	"log/slog"
	"strings"

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

func (s *Service) ListLogs(ctx context.Context, params auditrepo.ListParams) ([]auditrepo.LogRecord, int64, error) {
	return s.repo.List(ctx, params)
}

func (s *Service) GetSummary(ctx context.Context) (auditrepo.Summary, error) {
	return s.repo.Summary(ctx)
}

func (s *Service) ListActors(ctx context.Context, search string) ([]auditrepo.ActorOption, error) {
	return s.repo.ListActors(ctx, search)
}

func (s *Service) ExportCSV(ctx context.Context, params auditrepo.ListParams) ([]byte, error) {
	items, err := s.repo.ListForExport(ctx, params)
	if err != nil {
		return nil, err
	}

	builder := &strings.Builder{}
	writer := csv.NewWriter(builder)
	if err := writer.Write([]string{
		"timestamp",
		"user_name",
		"user_email",
		"module",
		"action",
		"resource",
		"resource_id",
		"ip_address",
		"old_value",
		"new_value",
	}); err != nil {
		return nil, err
	}

	for _, item := range items {
		if err := writer.Write([]string{
			item.CreatedAt.Format("2006-01-02 15:04:05 MST"),
			item.UserName,
			item.UserEmail,
			item.Module,
			item.Action,
			item.Resource,
			item.ResourceID,
			item.IPAddress,
			string(item.OldValue),
			string(item.NewValue),
		}); err != nil {
			return nil, err
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}

	return []byte(builder.String()), nil
}
