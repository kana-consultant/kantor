package hris

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	shareddto "github.com/kana-consultant/kantor/backend/internal/dto"
	hrisdto "github.com/kana-consultant/kantor/backend/internal/dto/hris"
	"github.com/kana-consultant/kantor/backend/internal/model"
	"github.com/kana-consultant/kantor/backend/internal/rbac"
	"github.com/kana-consultant/kantor/backend/internal/repository"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
	notificationsrepo "github.com/kana-consultant/kantor/backend/internal/repository/notifications"
)

var (
	ErrReimbursementNotFound          = errors.New("reimbursement not found")
	ErrReimbursementForbidden         = errors.New("reimbursement access is forbidden")
	ErrReimbursementInvalidState      = errors.New("reimbursement status transition is invalid")
	ErrReimbursementInvalidAttachment = errors.New("reimbursement attachment is invalid")
)

// ReimbursementStatusNotifier is called when a reimbursement status changes.
type ReimbursementStatusNotifier interface {
	SendReimbursementStatusNotification(ctx context.Context, reimbursementID string, newStatus string, reviewerNotes string)
}

type reimbursementsRepository interface {
	Create(ctx context.Context, params hrisrepo.CreateReimbursementParams) (model.Reimbursement, error)
	List(ctx context.Context, params hrisrepo.ListReimbursementsParams) ([]model.Reimbursement, int64, error)
	GetByID(ctx context.Context, reimbursementID string) (model.Reimbursement, error)
	Update(ctx context.Context, reimbursementID string, params hrisrepo.UpdateReimbursementParams) (model.Reimbursement, error)
	Delete(ctx context.Context, reimbursementID string) error
	AddAttachments(ctx context.Context, reimbursementID string, attachments []string) (model.Reimbursement, error)
	ApplyManagerReview(ctx context.Context, reimbursementID string, params hrisrepo.ReviewReimbursementParams) (model.Reimbursement, error)
	MarkPaid(ctx context.Context, reimbursementID string, actorID string, notes *string) (model.Reimbursement, error)
	Summary(ctx context.Context, month int, year int, employeeID string, department string) (model.ReimbursementSummary, error)
}

type reimbursementsEmployeesRepository interface {
	GetEmployeeByID(ctx context.Context, employeeID string) (model.Employee, error)
	GetEmployeeByUserID(ctx context.Context, userID string) (model.Employee, error)
}

type reimbursementsAuthRepository interface {
	ListUserIDsByPermission(ctx context.Context, permissionID string) ([]string, error)
}

type reimbursementsNotificationsService interface {
	CreateMany(ctx context.Context, params []notificationsrepo.CreateParams) error
}

type ReimbursementsService struct {
	repo                 reimbursementsRepository
	employeesRepo        reimbursementsEmployeesRepository
	authRepo             reimbursementsAuthRepository
	notificationsService reimbursementsNotificationsService
	waNotifier           ReimbursementStatusNotifier
	financeService       *FinanceService
}

func NewReimbursementsService(
	repo reimbursementsRepository,
	employeesRepo reimbursementsEmployeesRepository,
	authRepo reimbursementsAuthRepository,
	notificationsService reimbursementsNotificationsService,
	financeService *FinanceService,
) *ReimbursementsService {
	return &ReimbursementsService{
		repo:                 repo,
		employeesRepo:        employeesRepo,
		authRepo:             authRepo,
		notificationsService: notificationsService,
		financeService:       financeService,
	}
}

func (s *ReimbursementsService) SetWANotifier(n ReimbursementStatusNotifier) {
	s.waNotifier = n
}

func (s *ReimbursementsService) Create(ctx context.Context, request hrisdto.CreateReimbursementRequest, actorID string, perms *rbac.CachedPermissions) (model.Reimbursement, error) {
	transactionDate, err := shareddto.ParseDateOnly(request.TransactionDate)
	if err != nil {
		return model.Reimbursement{}, err
	}
	employee, err := s.employeesRepo.GetEmployeeByID(ctx, request.EmployeeID)
	if err != nil {
		return model.Reimbursement{}, mapReimbursementError(err)
	}

	if !canViewAllReimbursements(perms) {
		if employee.UserID == nil || strings.TrimSpace(*employee.UserID) != actorID {
			return model.Reimbursement{}, ErrReimbursementForbidden
		}
	}

	item, err := s.repo.Create(ctx, hrisrepo.CreateReimbursementParams{
		EmployeeID:      request.EmployeeID,
		Title:           strings.TrimSpace(request.Title),
		Category:        strings.TrimSpace(request.Category),
		Amount:          request.Amount,
		TransactionDate: transactionDate,
		Description:     strings.TrimSpace(request.Description),
		SubmittedBy:     actorID,
	})
	if err != nil {
		return model.Reimbursement{}, mapReimbursementError(err)
	}

	if err := s.notifyApproversForSubmission(ctx, item); err != nil {
		return model.Reimbursement{}, err
	}

	return item, nil
}

func (s *ReimbursementsService) List(ctx context.Context, query hrisdto.ListReimbursementsQuery, actorID string, perms *rbac.CachedPermissions) ([]model.Reimbursement, int64, int, int, error) {
	page := query.Page
	if page < 1 {
		page = 1
	}
	perPage := query.PerPage
	if perPage < 1 {
		perPage = 20
	}

	employeeID := strings.TrimSpace(query.EmployeeID)
	if !canViewAllReimbursements(perms) {
		employee, err := s.employeesRepo.GetEmployeeByUserID(ctx, actorID)
		if err != nil {
			return nil, 0, page, perPage, mapReimbursementError(err)
		}
		// Non-admin users can only see their own reimbursements.
		// If a specific employee filter was requested but it's not their own, return forbidden.
		if employeeID != "" && employeeID != employee.ID {
			return nil, 0, page, perPage, ErrReimbursementForbidden
		}
		employeeID = employee.ID
	}

	items, total, err := s.repo.List(ctx, hrisrepo.ListReimbursementsParams{
		Page:       page,
		PerPage:    perPage,
		Status:     strings.TrimSpace(query.Status),
		EmployeeID: employeeID,
		Department: "",
		Month:      query.Month,
		Year:       query.Year,
		SortBy:     strings.TrimSpace(query.SortBy),
		SortOrder:  strings.TrimSpace(query.SortOrder),
	})
	return items, total, page, perPage, err
}

func (s *ReimbursementsService) Get(ctx context.Context, reimbursementID string, actorID string, perms *rbac.CachedPermissions) (model.Reimbursement, error) {
	item, err := s.repo.GetByID(ctx, reimbursementID)
	if err != nil {
		return model.Reimbursement{}, mapReimbursementError(err)
	}
	if err := s.ensureAccessible(ctx, item, actorID, perms); err != nil {
		return model.Reimbursement{}, err
	}
	return item, nil
}

func (s *ReimbursementsService) Update(ctx context.Context, reimbursementID string, request hrisdto.UpdateReimbursementRequest, actorID string, perms *rbac.CachedPermissions) (model.Reimbursement, error) {
	transactionDate, err := shareddto.ParseDateOnly(request.TransactionDate)
	if err != nil {
		return model.Reimbursement{}, err
	}
	item, err := s.repo.GetByID(ctx, reimbursementID)
	if err != nil {
		return model.Reimbursement{}, mapReimbursementError(err)
	}
	if err := s.ensureEditable(ctx, item, actorID, perms); err != nil {
		return model.Reimbursement{}, err
	}
	if item.Status != "submitted" {
		return model.Reimbursement{}, ErrReimbursementInvalidState
	}
	keptAttachments := item.Attachments
	if request.KeptAttachments != nil {
		keptAttachments, err = filterKeptAttachments(item.Attachments, request.KeptAttachments)
		if err != nil {
			return model.Reimbursement{}, err
		}
	}
	updated, err := s.repo.Update(ctx, reimbursementID, hrisrepo.UpdateReimbursementParams{
		Title:           strings.TrimSpace(request.Title),
		Category:        strings.TrimSpace(request.Category),
		Amount:          request.Amount,
		TransactionDate: transactionDate,
		Description:     strings.TrimSpace(request.Description),
		Attachments:     keptAttachments,
	})
	if err != nil {
		return model.Reimbursement{}, mapReimbursementError(err)
	}
	return updated, nil
}

func (s *ReimbursementsService) Delete(ctx context.Context, reimbursementID string, actorID string, perms *rbac.CachedPermissions) error {
	item, err := s.repo.GetByID(ctx, reimbursementID)
	if err != nil {
		return mapReimbursementError(err)
	}
	if err := s.ensureEditable(ctx, item, actorID, perms); err != nil {
		return err
	}
	if item.Status != "submitted" {
		return ErrReimbursementInvalidState
	}
	return mapReimbursementError(s.repo.Delete(ctx, reimbursementID))
}

func (s *ReimbursementsService) AddAttachments(ctx context.Context, reimbursementID string, attachments []string, actorID string, perms *rbac.CachedPermissions) (model.Reimbursement, error) {
	item, err := s.repo.GetByID(ctx, reimbursementID)
	if err != nil {
		return model.Reimbursement{}, mapReimbursementError(err)
	}
	if err := s.ensureEditable(ctx, item, actorID, perms); err != nil {
		return model.Reimbursement{}, err
	}
	updated, err := s.repo.AddAttachments(ctx, reimbursementID, attachments)
	return updated, mapReimbursementError(err)
}

func (s *ReimbursementsService) ManagerReview(ctx context.Context, reimbursementID string, request hrisdto.ReviewReimbursementRequest, actorID string, perms *rbac.CachedPermissions) (model.Reimbursement, error) {
	if !canApproveReimbursements(perms) {
		return model.Reimbursement{}, ErrReimbursementForbidden
	}
	updated, err := s.repo.ApplyManagerReview(ctx, reimbursementID, hrisrepo.ReviewReimbursementParams{
		Decision: strings.TrimSpace(request.Decision),
		ActorID:  actorID,
		Notes:    request.Notes,
	})
	if err != nil {
		return model.Reimbursement{}, mapReimbursementError(err)
	}
	if err := s.notifyRequester(ctx, updated, "reimbursement.reviewed", "Reimbursement review updated"); err != nil {
		return model.Reimbursement{}, err
	}

	// UC-6: WA notification
	if s.waNotifier != nil {
		notes := ""
		if request.Notes != nil {
			notes = *request.Notes
		}
		go s.waNotifier.SendReimbursementStatusNotification(context.Background(), reimbursementID, updated.Status, notes)
	}

	return updated, nil
}

func (s *ReimbursementsService) FinanceReview(ctx context.Context, reimbursementID string, request hrisdto.ReviewReimbursementRequest, actorID string, perms *rbac.CachedPermissions) (model.Reimbursement, error) {
	return s.ManagerReview(ctx, reimbursementID, request, actorID, perms)
}

func (s *ReimbursementsService) MarkPaid(ctx context.Context, reimbursementID string, notes *string, actorID string, perms *rbac.CachedPermissions) (model.Reimbursement, error) {
	if !canMarkPaidReimbursements(perms) {
		return model.Reimbursement{}, ErrReimbursementForbidden
	}

	// Use a transaction so MarkPaid + finance record are atomic.
	db := repository.DB(ctx, nil)
	if db == nil {
		return model.Reimbursement{}, fmt.Errorf("no database connection in context")
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return model.Reimbursement{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	txCtx := repository.WithConn(ctx, tx)

	updated, err := s.repo.MarkPaid(txCtx, reimbursementID, actorID, notes)
	if err != nil {
		return model.Reimbursement{}, mapReimbursementError(err)
	}

	if s.financeService != nil {
		recordDate := time.Now()
		if updated.PaidAt != nil {
			recordDate = *updated.PaidAt
		}
		if finErr := s.financeService.RecordOutcome(txCtx, "reimbursement", updated.Amount, "Reimbursement: "+updated.Title, recordDate, actorID); finErr != nil {
			return model.Reimbursement{}, fmt.Errorf("record finance entry: %w", finErr)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return model.Reimbursement{}, fmt.Errorf("commit transaction: %w", err)
	}

	if err := s.notifyRequester(ctx, updated, "reimbursement.paid", "Reimbursement has been marked as paid"); err != nil {
		return model.Reimbursement{}, err
	}

	// UC-6: WA notification
	if s.waNotifier != nil {
		n := ""
		if notes != nil {
			n = *notes
		}
		go s.waNotifier.SendReimbursementStatusNotification(context.Background(), reimbursementID, updated.Status, n)
	}

	return updated, nil
}

func (s *ReimbursementsService) Summary(ctx context.Context, month int, year int, actorID string, perms *rbac.CachedPermissions) (model.ReimbursementSummary, error) {
	employeeID := ""
	if month < 1 {
		month = int(time.Now().Month())
	}
	if year < 1 {
		year = time.Now().Year()
	}

	if !canViewAllReimbursements(perms) {
		employee, err := s.employeesRepo.GetEmployeeByUserID(ctx, actorID)
		if err != nil {
			return model.ReimbursementSummary{}, mapReimbursementError(err)
		}
		employeeID = employee.ID
	}

	return s.repo.Summary(ctx, month, year, employeeID, "")
}

func (s *ReimbursementsService) ensureAccessible(ctx context.Context, item model.Reimbursement, actorID string, perms *rbac.CachedPermissions) error {
	if canViewAllReimbursements(perms) {
		return nil
	}
	employee, err := s.employeesRepo.GetEmployeeByUserID(ctx, actorID)
	if err != nil {
		return mapReimbursementError(err)
	}
	if employee.ID != item.EmployeeID {
		return ErrReimbursementForbidden
	}
	return nil
}

func (s *ReimbursementsService) ensureEditable(ctx context.Context, item model.Reimbursement, actorID string, perms *rbac.CachedPermissions) error {
	if canViewAllReimbursements(perms) {
		return nil
	}
	employee, err := s.employeesRepo.GetEmployeeByUserID(ctx, actorID)
	if err != nil {
		return mapReimbursementError(err)
	}
	if employee.ID != item.EmployeeID {
		return ErrReimbursementForbidden
	}
	if item.Status != "submitted" {
		return ErrReimbursementForbidden
	}
	return nil
}

func (s *ReimbursementsService) notifyApproversForSubmission(ctx context.Context, item model.Reimbursement) error {
	recipients, err := s.authRepo.ListUserIDsByPermission(ctx, "hris:reimbursement:approve")
	if err != nil {
		return err
	}
	return s.sendNotifications(ctx, recipients, "reimbursement.submitted", "New reimbursement submitted", item.Title, "reimbursement", &item.ID)
}

func (s *ReimbursementsService) notifyRequester(ctx context.Context, item model.Reimbursement, notificationType string, title string) error {
	requesterID := item.SubmittedBy
	return s.sendNotifications(ctx, []string{requesterID}, notificationType, title, item.Title+" is now "+item.Status, "reimbursement", &item.ID)
}

func (s *ReimbursementsService) sendNotifications(ctx context.Context, userIDs []string, notificationType string, title string, message string, referenceType string, referenceID *string) error {
	params := make([]notificationsrepo.CreateParams, 0, len(userIDs))
	for _, userID := range uniqueIDs(userIDs) {
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
	return s.notificationsService.CreateMany(ctx, params)
}

func uniqueIDs(userIDs []string) []string {
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

func filterKeptAttachments(existing []string, requested []string) ([]string, error) {
	allowed := make(map[string]struct{}, len(existing))
	for _, attachment := range existing {
		normalized := normalizeAttachmentPath(attachment)
		if normalized == "" {
			continue
		}
		allowed[normalized] = struct{}{}
	}

	filtered := make([]string, 0, len(requested))
	seen := make(map[string]struct{}, len(requested))
	for _, attachment := range requested {
		normalized := normalizeAttachmentPath(attachment)
		if normalized == "" {
			continue
		}
		if _, ok := allowed[normalized]; !ok {
			return nil, ErrReimbursementInvalidAttachment
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		filtered = append(filtered, normalized)
	}

	return filtered, nil
}

func normalizeAttachmentPath(value string) string {
	return filepath.ToSlash(strings.TrimSpace(value))
}

func mapReimbursementError(err error) error {
	switch {
	case errors.Is(err, hrisrepo.ErrReimbursementNotFound):
		return ErrReimbursementNotFound
	case errors.Is(err, hrisrepo.ErrEmployeeNotFound):
		return ErrEmployeeNotFound
	default:
		return err
	}
}

func canViewAllReimbursements(perms *rbac.CachedPermissions) bool {
	return rbac.CanViewAll(perms, "hris:reimbursement:view_all")
}

func canApproveReimbursements(perms *rbac.CachedPermissions) bool {
	return perms != nil && (perms.IsSuperAdmin || perms.Permissions["hris:reimbursement:approve"])
}

func canMarkPaidReimbursements(perms *rbac.CachedPermissions) bool {
	return perms != nil && (perms.IsSuperAdmin || perms.Permissions["hris:reimbursement:mark_paid"])
}
