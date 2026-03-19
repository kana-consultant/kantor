package hris

import (
	"context"
	"errors"
	"strings"
	"time"

	hrisdto "github.com/kana-consultant/kantor/backend/internal/dto/hris"
	"github.com/kana-consultant/kantor/backend/internal/model"
	authrepo "github.com/kana-consultant/kantor/backend/internal/repository/auth"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
	notificationsrepo "github.com/kana-consultant/kantor/backend/internal/repository/notifications"
	notificationsservice "github.com/kana-consultant/kantor/backend/internal/service/notifications"
)

var (
	ErrReimbursementNotFound     = errors.New("reimbursement not found")
	ErrReimbursementForbidden    = errors.New("reimbursement access is forbidden")
	ErrReimbursementInvalidState = errors.New("reimbursement status transition is invalid")
)

// ReimbursementStatusNotifier is called when a reimbursement status changes.
type ReimbursementStatusNotifier interface {
	SendReimbursementStatusNotification(ctx context.Context, reimbursementID string, newStatus string, reviewerNotes string)
}

type ReimbursementsService struct {
	repo                 *hrisrepo.ReimbursementsRepository
	employeesRepo        *hrisrepo.EmployeesRepository
	authRepo             *authrepo.Repository
	notificationsService *notificationsservice.Service
	waNotifier           ReimbursementStatusNotifier
}

func NewReimbursementsService(
	repo *hrisrepo.ReimbursementsRepository,
	employeesRepo *hrisrepo.EmployeesRepository,
	authRepo *authrepo.Repository,
	notificationsService *notificationsservice.Service,
) *ReimbursementsService {
	return &ReimbursementsService{
		repo:                 repo,
		employeesRepo:        employeesRepo,
		authRepo:             authRepo,
		notificationsService: notificationsService,
	}
}

func (s *ReimbursementsService) SetWANotifier(n ReimbursementStatusNotifier) {
	s.waNotifier = n
}

func (s *ReimbursementsService) Create(ctx context.Context, request hrisdto.CreateReimbursementRequest, actorID string, roles []string) (model.Reimbursement, error) {
	employee, err := s.employeesRepo.GetEmployeeByID(ctx, request.EmployeeID)
	if err != nil {
		return model.Reimbursement{}, mapReimbursementError(err)
	}

	if isStaffHRIS(roles) {
		if employee.UserID == nil || strings.TrimSpace(*employee.UserID) != actorID {
			return model.Reimbursement{}, ErrReimbursementForbidden
		}
	}

	item, err := s.repo.Create(ctx, hrisrepo.CreateReimbursementParams{
		EmployeeID:      request.EmployeeID,
		Title:           strings.TrimSpace(request.Title),
		Category:        strings.TrimSpace(request.Category),
		Amount:          request.Amount,
		TransactionDate: request.TransactionDate,
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

func (s *ReimbursementsService) List(ctx context.Context, query hrisdto.ListReimbursementsQuery, actorID string, roles []string) ([]model.Reimbursement, int64, int, int, error) {
	page := query.Page
	if page < 1 {
		page = 1
	}
	perPage := query.PerPage
	if perPage < 1 {
		perPage = 20
	}

	employeeID := strings.TrimSpace(query.EmployeeID)
	department := ""
	if isStaffHRIS(roles) {
		employee, err := s.employeesRepo.GetEmployeeByUserID(ctx, actorID)
		if err != nil {
			return nil, 0, page, perPage, mapReimbursementError(err)
		}
		employeeID = employee.ID
	}
	if isManagerHRIS(roles) && !isAdminHRIS(roles) && !isSuperAdmin(roles) {
		employee, err := s.employeesRepo.GetEmployeeByUserID(ctx, actorID)
		if err != nil {
			return nil, 0, page, perPage, mapReimbursementError(err)
		}
		if employee.Department != nil {
			department = strings.TrimSpace(*employee.Department)
		}
	}

	items, total, err := s.repo.List(ctx, hrisrepo.ListReimbursementsParams{
		Page:       page,
		PerPage:    perPage,
		Status:     strings.TrimSpace(query.Status),
		EmployeeID: employeeID,
		Department: department,
		Month:      query.Month,
		Year:       query.Year,
	})
	return items, total, page, perPage, err
}

func (s *ReimbursementsService) Get(ctx context.Context, reimbursementID string, actorID string, roles []string) (model.Reimbursement, error) {
	item, err := s.repo.GetByID(ctx, reimbursementID)
	if err != nil {
		return model.Reimbursement{}, mapReimbursementError(err)
	}
	if err := s.ensureAccessible(ctx, item, actorID, roles); err != nil {
		return model.Reimbursement{}, err
	}
	return item, nil
}

func (s *ReimbursementsService) AddAttachments(ctx context.Context, reimbursementID string, attachments []string, actorID string, roles []string) (model.Reimbursement, error) {
	item, err := s.repo.GetByID(ctx, reimbursementID)
	if err != nil {
		return model.Reimbursement{}, mapReimbursementError(err)
	}
	if err := s.ensureEditable(ctx, item, actorID, roles); err != nil {
		return model.Reimbursement{}, err
	}
	updated, err := s.repo.AddAttachments(ctx, reimbursementID, attachments)
	return updated, mapReimbursementError(err)
}

func (s *ReimbursementsService) ManagerReview(ctx context.Context, reimbursementID string, request hrisdto.ReviewReimbursementRequest, actorID string, roles []string) (model.Reimbursement, error) {
	if !isManagerHRIS(roles) && !isAdminHRIS(roles) && !isSuperAdmin(roles) {
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

func (s *ReimbursementsService) FinanceReview(ctx context.Context, reimbursementID string, request hrisdto.ReviewReimbursementRequest, actorID string, roles []string) (model.Reimbursement, error) {
	return s.ManagerReview(ctx, reimbursementID, request, actorID, roles)
}

func (s *ReimbursementsService) MarkPaid(ctx context.Context, reimbursementID string, notes *string, actorID string, roles []string) (model.Reimbursement, error) {
	if !isManagerHRIS(roles) && !isAdminHRIS(roles) && !isSuperAdmin(roles) {
		return model.Reimbursement{}, ErrReimbursementForbidden
	}
	updated, err := s.repo.MarkPaid(ctx, reimbursementID, actorID, notes)
	if err != nil {
		return model.Reimbursement{}, mapReimbursementError(err)
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

func (s *ReimbursementsService) Summary(ctx context.Context, month int, year int, actorID string, roles []string) (model.ReimbursementSummary, error) {
	employeeID := ""
	department := ""
	if month < 1 {
		month = int(time.Now().Month())
	}
	if year < 1 {
		year = time.Now().Year()
	}

	if isStaffHRIS(roles) {
		employee, err := s.employeesRepo.GetEmployeeByUserID(ctx, actorID)
		if err != nil {
			return model.ReimbursementSummary{}, mapReimbursementError(err)
		}
		employeeID = employee.ID
	}
	if isManagerHRIS(roles) && !isAdminHRIS(roles) && !isSuperAdmin(roles) {
		employee, err := s.employeesRepo.GetEmployeeByUserID(ctx, actorID)
		if err != nil {
			return model.ReimbursementSummary{}, mapReimbursementError(err)
		}
		if employee.Department != nil {
			department = strings.TrimSpace(*employee.Department)
		}
	}

	return s.repo.Summary(ctx, month, year, employeeID, department)
}

func (s *ReimbursementsService) ensureAccessible(ctx context.Context, item model.Reimbursement, actorID string, roles []string) error {
	if isSuperAdmin(roles) || isAdminHRIS(roles) {
		return nil
	}
	if isStaffHRIS(roles) {
		employee, err := s.employeesRepo.GetEmployeeByUserID(ctx, actorID)
		if err != nil {
			return mapReimbursementError(err)
		}
		if employee.ID != item.EmployeeID {
			return ErrReimbursementForbidden
		}
		return nil
	}
	if isManagerHRIS(roles) {
		return nil
	}
	return nil
}

func (s *ReimbursementsService) ensureEditable(ctx context.Context, item model.Reimbursement, actorID string, roles []string) error {
	if isSuperAdmin(roles) || isAdminHRIS(roles) {
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
	managers, err := s.authRepo.ListUserIDsByRole(ctx, "manager", "hris")
	if err != nil {
		return err
	}
	admins, err := s.authRepo.ListUserIDsByRole(ctx, "admin", "hris")
	if err != nil {
		return err
	}
	superAdmins, err := s.authRepo.ListUserIDsByRole(ctx, "super_admin", "")
	if err != nil {
		return err
	}
	return s.sendNotifications(ctx, uniqueIDs(append(append(managers, admins...), superAdmins...)), "reimbursement.submitted", "New reimbursement submitted", item.Title, "reimbursement", &item.ID)
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

func isSuperAdmin(roles []string) bool {
	return containsRole(roles, "super_admin")
}

func isAdminHRIS(roles []string) bool {
	return containsRole(roles, "admin:hris")
}

func isManagerHRIS(roles []string) bool {
	return containsRole(roles, "manager:hris")
}

func isStaffHRIS(roles []string) bool {
	return containsRole(roles, "staff:hris")
}

func containsRole(roles []string, target string) bool {
	for _, role := range roles {
		if strings.EqualFold(strings.TrimSpace(role), target) {
			return true
		}
	}
	return false
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
