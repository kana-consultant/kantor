package hris

import (
	"context"
	"errors"
	"strings"

	hrisdto "github.com/kana-consultant/kantor/backend/internal/dto/hris"
	"github.com/kana-consultant/kantor/backend/internal/model"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
)

var (
	ErrEmployeeNotFound        = errors.New("employee not found")
	ErrEmployeeEmailExists     = errors.New("employee email already exists")
	ErrEmployeeUserLinkedTwice = errors.New("user account is already linked to another employee")
)

type EmployeesService struct {
	repo     *hrisrepo.EmployeesRepository
	authRepo userFieldsSyncer
}

// userFieldsSyncer syncs employee changes back to the users table.
type userFieldsSyncer interface {
	UpdateUserFields(ctx context.Context, userID string, fullName string, email string) error
}

func NewEmployeesService(repo *hrisrepo.EmployeesRepository) *EmployeesService {
	return &EmployeesService{repo: repo}
}

func (s *EmployeesService) SetAuthRepo(syncer userFieldsSyncer) {
	s.authRepo = syncer
}

func (s *EmployeesService) CreateEmployee(ctx context.Context, request hrisdto.CreateEmployeeRequest) (model.Employee, error) {
	employee, err := s.repo.CreateEmployee(ctx, hrisrepo.UpsertEmployeeParams{
		FullName:         strings.TrimSpace(request.FullName),
		Email:            strings.ToLower(strings.TrimSpace(request.Email)),
		Phone:            trimOptionalString(request.Phone),
		Position:         strings.TrimSpace(request.Position),
		Department:       trimOptionalString(request.Department),
		DateJoined:       request.DateJoined,
		EmploymentStatus: strings.TrimSpace(request.EmploymentStatus),
		Address:          trimOptionalString(request.Address),
		EmergencyContact: trimOptionalString(request.EmergencyContact),
		AvatarURL:        trimOptionalString(request.AvatarURL),
	})
	if err != nil {
		return model.Employee{}, mapEmployeeError(err)
	}

	return employee, nil
}

func (s *EmployeesService) ListEmployees(ctx context.Context, query hrisdto.ListEmployeesQuery) ([]model.Employee, int64, int, int, error) {
	page := query.Page
	if page <= 0 {
		page = 1
	}

	perPage := query.PerPage
	if perPage <= 0 {
		perPage = 10
	}

	employees, total, err := s.repo.ListEmployees(ctx, hrisrepo.ListEmployeesParams{
		Page:             page,
		PerPage:          perPage,
		Search:           strings.TrimSpace(query.Search),
		Department:       strings.TrimSpace(query.Department),
		EmploymentStatus: strings.TrimSpace(query.EmploymentStatus),
	})
	if err != nil {
		return nil, 0, 0, 0, err
	}

	return employees, total, page, perPage, nil
}

func (s *EmployeesService) GetEmployee(ctx context.Context, employeeID string) (model.Employee, error) {
	employee, err := s.repo.GetEmployeeByID(ctx, employeeID)
	if err != nil {
		return model.Employee{}, mapEmployeeError(err)
	}

	return employee, nil
}

func (s *EmployeesService) UpdateEmployee(ctx context.Context, employeeID string, request hrisdto.UpdateEmployeeRequest) (model.Employee, error) {
	employee, err := s.repo.UpdateEmployee(ctx, employeeID, hrisrepo.UpsertEmployeeParams{
		FullName:         strings.TrimSpace(request.FullName),
		Email:            strings.ToLower(strings.TrimSpace(request.Email)),
		Phone:            trimOptionalString(request.Phone),
		Position:         strings.TrimSpace(request.Position),
		Department:       trimOptionalString(request.Department),
		DateJoined:       request.DateJoined,
		EmploymentStatus: strings.TrimSpace(request.EmploymentStatus),
		Address:          trimOptionalString(request.Address),
		EmergencyContact: trimOptionalString(request.EmergencyContact),
		AvatarURL:        trimOptionalString(request.AvatarURL),
	})
	if err != nil {
		return model.Employee{}, mapEmployeeError(err)
	}

	// Sync full_name and email back to users table if linked
	if employee.UserID != nil && s.authRepo != nil {
		_ = s.authRepo.UpdateUserFields(ctx, *employee.UserID, employee.FullName, employee.Email)
	}

	return employee, nil
}

func (s *EmployeesService) DeleteEmployee(ctx context.Context, employeeID string) error {
	return mapEmployeeError(s.repo.DeleteEmployee(ctx, employeeID))
}

func mapEmployeeError(err error) error {
	switch {
	case errors.Is(err, hrisrepo.ErrEmployeeNotFound):
		return ErrEmployeeNotFound
	case errors.Is(err, hrisrepo.ErrEmployeeEmailExists):
		return ErrEmployeeEmailExists
	case errors.Is(err, hrisrepo.ErrEmployeeUserAlreadyUsed):
		return ErrEmployeeUserLinkedTwice
	default:
		return err
	}
}
