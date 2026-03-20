package operational

import (
	"context"
	"errors"
	"strings"
	"time"

	operationaldto "github.com/kana-consultant/kantor/backend/internal/dto/operational"
	"github.com/kana-consultant/kantor/backend/internal/model"
	operationalrepo "github.com/kana-consultant/kantor/backend/internal/repository/operational"
)

var (
	ErrConsentRequired        = errors.New("tracking consent is required before activity can be recorded")
	ErrTrackerSessionNotFound = errors.New("activity session not found")
	ErrDomainCategoryNotFound = errors.New("domain category not found")
)

type trackerRepository interface {
	GetConsent(ctx context.Context, userID string) (model.ActivityConsent, error)
	UpsertConsent(ctx context.Context, userID string, consented bool, ipAddress string, now time.Time) (model.ActivityConsent, error)
	StartSession(ctx context.Context, userID string, startedAt time.Time) (model.ActivitySession, error)
	EndSession(ctx context.Context, userID string, sessionID string, endedAt time.Time) (model.ActivitySession, error)
	RecordHeartbeat(ctx context.Context, params operationalrepo.TrackerHeartbeatParams) (model.ActivityEntry, model.ActivitySession, error)
	GetActivityOverview(ctx context.Context, userID string, activityRange operationalrepo.TrackerActivityRange) (model.TrackerActivityOverview, error)
	GetTeamActivity(ctx context.Context, activityRange operationalrepo.TrackerActivityRange, userID *string) (model.TrackerTeamOverview, error)
	GetDailySummary(ctx context.Context, date time.Time) (model.TrackerDailySummary, error)
	ListDomainCategories(ctx context.Context) ([]model.DomainCategory, error)
	ListConsentAudit(ctx context.Context) ([]model.TrackerConsentAudit, error)
	CreateDomainCategory(ctx context.Context, params operationalrepo.UpsertDomainCategoryParams) (model.DomainCategory, error)
	UpdateDomainCategory(ctx context.Context, domainID string, params operationalrepo.UpsertDomainCategoryParams) (model.DomainCategory, error)
	DeleteDomainCategory(ctx context.Context, domainID string) error
	PurgeOldSessions(ctx context.Context, cutoff time.Time) (int64, error)
}

type TrackerService struct {
	repo          trackerRepository
	retentionDays int
}

type TrackerBatchResult struct {
	Processed int `json:"processed"`
	Skipped   int `json:"skipped"`
}

func NewTrackerService(repo trackerRepository, retentionDays int) *TrackerService {
	if retentionDays <= 0 {
		retentionDays = 90
	}
	return &TrackerService{repo: repo, retentionDays: retentionDays}
}

func (s *TrackerService) GetConsent(ctx context.Context, userID string) (model.ActivityConsent, error) {
	consent, err := s.repo.GetConsent(ctx, userID)
	if errors.Is(err, operationalrepo.ErrTrackerConsentNotFound) {
		return model.ActivityConsent{
			UserID:    userID,
			Consented: false,
		}, nil
	}
	return consent, err
}

func (s *TrackerService) GiveConsent(ctx context.Context, userID string, ipAddress string, now time.Time) (model.ActivityConsent, error) {
	return s.repo.UpsertConsent(ctx, userID, true, ipAddress, now)
}

func (s *TrackerService) RevokeConsent(ctx context.Context, userID string, ipAddress string, now time.Time) (model.ActivityConsent, error) {
	return s.repo.UpsertConsent(ctx, userID, false, ipAddress, now)
}

func (s *TrackerService) StartSession(ctx context.Context, userID string, now time.Time) (model.ActivitySession, error) {
	if err := s.requireConsent(ctx, userID); err != nil {
		return model.ActivitySession{}, err
	}
	return s.repo.StartSession(ctx, userID, now)
}

func (s *TrackerService) EndSession(ctx context.Context, userID string, sessionID string, now time.Time) (model.ActivitySession, error) {
	session, err := s.repo.EndSession(ctx, userID, sessionID, now)
	if errors.Is(err, operationalrepo.ErrTrackerSessionNotFound) {
		return model.ActivitySession{}, ErrTrackerSessionNotFound
	}
	return session, err
}

func (s *TrackerService) RecordHeartbeat(ctx context.Context, userID string, request operationaldto.TrackerHeartbeatRequest) (model.ActivityEntry, model.ActivitySession, error) {
	if err := s.requireConsent(ctx, userID); err != nil {
		return model.ActivityEntry{}, model.ActivitySession{}, err
	}

	entry, session, err := s.repo.RecordHeartbeat(ctx, operationalrepo.TrackerHeartbeatParams{
		SessionID: request.SessionID,
		UserID:    userID,
		URL:       strings.TrimSpace(request.URL),
		Domain:    strings.ToLower(strings.TrimSpace(request.Domain)),
		PageTitle: request.PageTitle,
		IsIdle:    request.IsIdle,
		Timestamp: request.Timestamp,
	})
	if errors.Is(err, operationalrepo.ErrTrackerSessionNotFound) {
		return model.ActivityEntry{}, model.ActivitySession{}, ErrTrackerSessionNotFound
	}
	return entry, session, err
}

func (s *TrackerService) RecordBatch(ctx context.Context, userID string, request operationaldto.TrackerBatchEntriesRequest) (TrackerBatchResult, error) {
	if err := s.requireConsent(ctx, userID); err != nil {
		return TrackerBatchResult{}, err
	}

	entries := append([]operationaldto.TrackerHeartbeatRequest(nil), request.Entries...)
	sortHeartbeats(entries)

	result := TrackerBatchResult{}
	for _, entry := range entries {
		if _, _, err := s.RecordHeartbeat(ctx, userID, entry); err != nil {
			result.Skipped++
			continue
		}
		result.Processed++
	}
	return result, nil
}

func (s *TrackerService) GetMyActivity(ctx context.Context, userID string, dateFrom time.Time, dateTo time.Time) (model.TrackerActivityOverview, error) {
	return s.repo.GetActivityOverview(ctx, userID, normalizedRange(dateFrom, dateTo))
}

func (s *TrackerService) GetUserActivity(ctx context.Context, userID string, dateFrom time.Time, dateTo time.Time) (model.TrackerActivityOverview, error) {
	return s.repo.GetActivityOverview(ctx, userID, normalizedRange(dateFrom, dateTo))
}

func (s *TrackerService) GetTeamActivity(ctx context.Context, dateFrom time.Time, dateTo time.Time, userID *string) (model.TrackerTeamOverview, error) {
	return s.repo.GetTeamActivity(ctx, normalizedRange(dateFrom, dateTo), userID)
}

func (s *TrackerService) GetDailySummary(ctx context.Context, date time.Time) (model.TrackerDailySummary, error) {
	normalized := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	return s.repo.GetDailySummary(ctx, normalized)
}

func (s *TrackerService) ListDomainCategories(ctx context.Context) ([]model.DomainCategory, error) {
	return s.repo.ListDomainCategories(ctx)
}

func (s *TrackerService) ListConsentAudit(ctx context.Context) ([]model.TrackerConsentAudit, error) {
	return s.repo.ListConsentAudit(ctx)
}

func (s *TrackerService) CreateDomainCategory(ctx context.Context, request operationaldto.DomainCategoryRequest) (model.DomainCategory, error) {
	return s.repo.CreateDomainCategory(ctx, operationalrepo.UpsertDomainCategoryParams{
		DomainPattern: request.DomainPattern,
		Category:      request.Category,
		IsProductive:  request.IsProductive,
	})
}

func (s *TrackerService) UpdateDomainCategory(ctx context.Context, domainID string, request operationaldto.DomainCategoryRequest) (model.DomainCategory, error) {
	item, err := s.repo.UpdateDomainCategory(ctx, domainID, operationalrepo.UpsertDomainCategoryParams{
		DomainPattern: request.DomainPattern,
		Category:      request.Category,
		IsProductive:  request.IsProductive,
	})
	if errors.Is(err, operationalrepo.ErrDomainCategoryNotFound) {
		return model.DomainCategory{}, ErrDomainCategoryNotFound
	}
	return item, err
}

func (s *TrackerService) DeleteDomainCategory(ctx context.Context, domainID string) error {
	err := s.repo.DeleteDomainCategory(ctx, domainID)
	if errors.Is(err, operationalrepo.ErrDomainCategoryNotFound) {
		return ErrDomainCategoryNotFound
	}
	return err
}

func (s *TrackerService) PurgeOldData(ctx context.Context, now time.Time) (int64, error) {
	cutoff := now.AddDate(0, 0, -s.retentionDays)
	return s.repo.PurgeOldSessions(ctx, cutoff)
}

func (s *TrackerService) requireConsent(ctx context.Context, userID string) error {
	consent, err := s.GetConsent(ctx, userID)
	if err != nil {
		return err
	}
	if !consent.Consented {
		return ErrConsentRequired
	}
	return nil
}

func normalizedRange(dateFrom time.Time, dateTo time.Time) operationalrepo.TrackerActivityRange {
	start := time.Date(dateFrom.Year(), dateFrom.Month(), dateFrom.Day(), 0, 0, 0, 0, dateFrom.Location())
	end := time.Date(dateTo.Year(), dateTo.Month(), dateTo.Day(), 0, 0, 0, 0, dateTo.Location())
	if end.Before(start) {
		end = start
	}
	return operationalrepo.TrackerActivityRange{DateFrom: start, DateTo: end}
}

func sortHeartbeats(entries []operationaldto.TrackerHeartbeatRequest) {
	if len(entries) < 2 {
		return
	}
	for i := 0; i < len(entries)-1; i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[j].Timestamp.Before(entries[i].Timestamp) {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}
}
