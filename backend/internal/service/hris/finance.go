package hris

import (
	"context"
	"encoding/csv"
	"errors"
	"strconv"
	"strings"

	hrisdto "github.com/kana-consultant/kantor/backend/internal/dto/hris"
	"github.com/kana-consultant/kantor/backend/internal/model"
	"github.com/kana-consultant/kantor/backend/internal/rbac"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
)

var (
	ErrFinanceCategoryNotFound = errors.New("finance category not found")
	ErrFinanceRecordNotFound   = errors.New("finance record not found")
	ErrFinanceCategoryExists   = errors.New("finance category already exists")
	ErrFinanceForbidden        = errors.New("finance record access is forbidden")
)

type financeRepository interface {
	CreateCategory(ctx context.Context, params hrisrepo.UpsertFinanceCategoryParams) (model.FinanceCategory, error)
	ListCategories(ctx context.Context, recordType string) ([]model.FinanceCategory, error)
	UpdateCategory(ctx context.Context, categoryID string, params hrisrepo.UpsertFinanceCategoryParams) (model.FinanceCategory, error)
	DeleteCategory(ctx context.Context, categoryID string) error
	CreateRecord(ctx context.Context, params hrisrepo.UpsertFinanceRecordParams) (model.FinanceRecord, error)
	ListRecords(ctx context.Context, params hrisrepo.ListFinanceRecordsParams) ([]model.FinanceRecord, int64, error)
	GetRecordByID(ctx context.Context, recordID string) (model.FinanceRecord, error)
	UpdateRecord(ctx context.Context, recordID string, params hrisrepo.UpsertFinanceRecordParams) (model.FinanceRecord, error)
	DeleteRecord(ctx context.Context, recordID string) error
	SubmitRecord(ctx context.Context, recordID string, actorID string) (model.FinanceRecord, error)
	ReviewRecord(ctx context.Context, recordID string, decision string, actorID string) (model.FinanceRecord, error)
	Summary(ctx context.Context, year int) (model.FinanceSummary, error)
	ListForExport(ctx context.Context, params hrisrepo.ListFinanceExportParams) ([]model.FinanceRecord, error)
}

type FinanceService struct {
	repo financeRepository
}

func NewFinanceService(repo financeRepository) *FinanceService {
	return &FinanceService{repo: repo}
}

func (s *FinanceService) CreateCategory(ctx context.Context, request hrisdto.CreateFinanceCategoryRequest) (model.FinanceCategory, error) {
	item, err := s.repo.CreateCategory(ctx, hrisrepo.UpsertFinanceCategoryParams{
		Name: strings.TrimSpace(request.Name),
		Type: strings.TrimSpace(request.Type),
	})
	return item, mapFinanceServiceError(err)
}

func (s *FinanceService) ListCategories(ctx context.Context, recordType string) ([]model.FinanceCategory, error) {
	return s.repo.ListCategories(ctx, recordType)
}

func (s *FinanceService) UpdateCategory(ctx context.Context, categoryID string, request hrisdto.UpdateFinanceCategoryRequest) (model.FinanceCategory, error) {
	item, err := s.repo.UpdateCategory(ctx, categoryID, hrisrepo.UpsertFinanceCategoryParams{
		Name: strings.TrimSpace(request.Name),
		Type: strings.TrimSpace(request.Type),
	})
	return item, mapFinanceServiceError(err)
}

func (s *FinanceService) DeleteCategory(ctx context.Context, categoryID string) error {
	return mapFinanceServiceError(s.repo.DeleteCategory(ctx, categoryID))
}

func (s *FinanceService) CreateRecord(ctx context.Context, request hrisdto.CreateFinanceRecordRequest, actorID string) (model.FinanceRecord, error) {
	item, err := s.repo.CreateRecord(ctx, hrisrepo.UpsertFinanceRecordParams{
		CategoryID:  strings.TrimSpace(request.CategoryID),
		Type:        strings.TrimSpace(request.Type),
		Amount:      request.Amount,
		Description: strings.TrimSpace(request.Description),
		RecordDate:  request.RecordDate,
		SubmittedBy: actorID,
	})
	return item, mapFinanceServiceError(err)
}

func (s *FinanceService) ListRecords(ctx context.Context, query hrisdto.ListFinanceRecordsQuery, actorID string, perms *rbac.CachedPermissions) ([]model.FinanceRecord, int64, int, int, error) {
	page := query.Page
	if page < 1 {
		page = 1
	}
	perPage := query.PerPage
	if perPage < 1 {
		perPage = 20
	}
	items, total, err := s.repo.ListRecords(ctx, hrisrepo.ListFinanceRecordsParams{
		Page:        page,
		PerPage:     perPage,
		Type:        strings.TrimSpace(query.Type),
		CategoryID:  strings.TrimSpace(query.CategoryID),
		Month:       query.Month,
		Year:        query.Year,
		Status:      strings.TrimSpace(query.Status),
		SubmittedBy: restrictFinanceSubmittedBy(actorID, perms),
	})
	return items, total, page, perPage, err
}

func (s *FinanceService) GetRecord(ctx context.Context, recordID string, actorID string, perms *rbac.CachedPermissions) (model.FinanceRecord, error) {
	item, err := s.repo.GetRecordByID(ctx, recordID)
	if err != nil {
		return item, mapFinanceServiceError(err)
	}
	if !canViewAllFinance(perms) && strings.TrimSpace(optionalString(item.SubmittedBy)) != strings.TrimSpace(actorID) {
		return model.FinanceRecord{}, ErrFinanceForbidden
	}
	return item, nil
}

func (s *FinanceService) UpdateRecord(ctx context.Context, recordID string, request hrisdto.UpdateFinanceRecordRequest, actorID string, perms *rbac.CachedPermissions) (model.FinanceRecord, error) {
	current, err := s.repo.GetRecordByID(ctx, recordID)
	if err != nil {
		return model.FinanceRecord{}, mapFinanceServiceError(err)
	}
	if !canViewAllFinance(perms) && strings.TrimSpace(optionalString(current.SubmittedBy)) != strings.TrimSpace(actorID) {
		return model.FinanceRecord{}, ErrFinanceForbidden
	}
	item, err := s.repo.UpdateRecord(ctx, recordID, hrisrepo.UpsertFinanceRecordParams{
		CategoryID:  strings.TrimSpace(request.CategoryID),
		Type:        strings.TrimSpace(request.Type),
		Amount:      request.Amount,
		Description: strings.TrimSpace(request.Description),
		RecordDate:  request.RecordDate,
		SubmittedBy: optionalString(current.SubmittedBy),
	})
	return item, mapFinanceServiceError(err)
}

func (s *FinanceService) DeleteRecord(ctx context.Context, recordID string, actorID string, perms *rbac.CachedPermissions) error {
	current, err := s.repo.GetRecordByID(ctx, recordID)
	if err != nil {
		return mapFinanceServiceError(err)
	}
	if !canViewAllFinance(perms) && strings.TrimSpace(optionalString(current.SubmittedBy)) != strings.TrimSpace(actorID) {
		return ErrFinanceForbidden
	}
	return mapFinanceServiceError(s.repo.DeleteRecord(ctx, recordID))
}

func (s *FinanceService) SubmitRecord(ctx context.Context, recordID string, actorID string, perms *rbac.CachedPermissions) (model.FinanceRecord, error) {
	current, err := s.repo.GetRecordByID(ctx, recordID)
	if err != nil {
		return model.FinanceRecord{}, mapFinanceServiceError(err)
	}
	if !canViewAllFinance(perms) && strings.TrimSpace(optionalString(current.SubmittedBy)) != strings.TrimSpace(actorID) {
		return model.FinanceRecord{}, ErrFinanceForbidden
	}
	item, err := s.repo.SubmitRecord(ctx, recordID, actorID)
	return item, mapFinanceServiceError(err)
}

func (s *FinanceService) ReviewRecord(ctx context.Context, recordID string, decision string, actorID string) (model.FinanceRecord, error) {
	item, err := s.repo.ReviewRecord(ctx, recordID, strings.TrimSpace(decision), actorID)
	return item, mapFinanceServiceError(err)
}

func (s *FinanceService) Summary(ctx context.Context, year int) (model.FinanceSummary, error) {
	return s.repo.Summary(ctx, year)
}

func (s *FinanceService) ExportCSV(ctx context.Context, year int, month int) ([]byte, error) {
	items, err := s.repo.ListForExport(ctx, hrisrepo.ListFinanceExportParams{Year: year, Month: month})
	if err != nil {
		return nil, err
	}

	builder := &strings.Builder{}
	writer := csv.NewWriter(builder)
	if err := writer.Write([]string{"id", "category", "type", "amount", "description", "record_date", "status"}); err != nil {
		return nil, err
	}
	for _, item := range items {
		if err := writer.Write([]string{
			item.ID,
			item.CategoryName,
			item.Type,
			strconv.FormatInt(item.Amount, 10),
			item.Description,
			item.RecordDate.Format("2006-01-02"),
			item.ApprovalStatus,
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

func mapFinanceServiceError(err error) error {
	switch {
	case errors.Is(err, hrisrepo.ErrFinanceCategoryNotFound):
		return ErrFinanceCategoryNotFound
	case errors.Is(err, hrisrepo.ErrFinanceRecordNotFound):
		return ErrFinanceRecordNotFound
	case errors.Is(err, hrisrepo.ErrFinanceCategoryExists):
		return ErrFinanceCategoryExists
	default:
		return err
	}
}

func canViewAllFinance(perms *rbac.CachedPermissions) bool {
	return rbac.CanViewAll(perms, "hris:finance:approve")
}

func restrictFinanceSubmittedBy(actorID string, perms *rbac.CachedPermissions) string {
	if canViewAllFinance(perms) {
		return ""
	}
	return strings.TrimSpace(actorID)
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
