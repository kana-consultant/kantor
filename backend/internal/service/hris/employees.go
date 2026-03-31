package hris

import (
	"context"
	"errors"
	"strings"

	shareddto "github.com/kana-consultant/kantor/backend/internal/dto"
	hrisdto "github.com/kana-consultant/kantor/backend/internal/dto/hris"
	"github.com/kana-consultant/kantor/backend/internal/model"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
)

var (
	ErrEmployeeNotFound        = errors.New("employee not found")
	ErrEmployeeEmailExists     = errors.New("employee email already exists")
	ErrEmployeeUserLinkedTwice = errors.New("user account is already linked to another employee")
)

type userFieldsSyncer interface {
	UpdateUserFields(ctx context.Context, userID string, fullName string, email string) error
	UpdateUserAvatar(ctx context.Context, userID string, avatarURL *string) error
}

type employeesRepository interface {
	CreateEmployee(ctx context.Context, params hrisrepo.UpsertEmployeeParams) (model.Employee, error)
	ListEmployees(ctx context.Context, params hrisrepo.ListEmployeesParams) ([]model.Employee, int64, error)
	GetEmployeeByID(ctx context.Context, employeeID string) (model.Employee, error)
	UpdateEmployee(ctx context.Context, employeeID string, params hrisrepo.UpsertEmployeeParams) (model.Employee, error)
	UpdateEmployeeAvatar(ctx context.Context, employeeID string, avatarURL *string) (model.Employee, error)
	DeleteEmployee(ctx context.Context, employeeID string) error
}

type EmployeesService struct {
	repo     employeesRepository
	authRepo userFieldsSyncer
}

func NewEmployeesService(repo employeesRepository) *EmployeesService {
	return &EmployeesService{repo: repo}
}

func (s *EmployeesService) SetAuthRepo(syncer userFieldsSyncer) {
	s.authRepo = syncer
}

func (s *EmployeesService) CreateEmployee(ctx context.Context, request hrisdto.CreateEmployeeRequest) (model.Employee, error) {
	dateJoined, err := shareddto.ParseDateOnly(request.DateJoined)
	if err != nil {
		return model.Employee{}, err
	}

	employee, err := s.repo.CreateEmployee(ctx, hrisrepo.UpsertEmployeeParams{
		FullName:          strings.TrimSpace(request.FullName),
		Email:             strings.ToLower(strings.TrimSpace(request.Email)),
		Phone:             trimOptionalString(request.Phone),
		Position:          strings.TrimSpace(request.Position),
		Department:        trimOptionalString(request.Department),
		DateJoined:        dateJoined,
		EmploymentStatus:  strings.TrimSpace(request.EmploymentStatus),
		Address:           trimOptionalString(request.Address),
		EmergencyContact:  trimOptionalString(request.EmergencyContact),
		AvatarURL:         trimOptionalString(request.AvatarURL),
		BankAccountNumber: trimOptionalString(request.BankAccountNumber),
		BankName:          trimOptionalString(request.BankName),
		LinkedInProfile:   trimOptionalString(request.LinkedInProfile),
		SSHKeys:           trimOptionalString(request.SSHKeys),
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
	dateJoined, err := shareddto.ParseDateOnly(request.DateJoined)
	if err != nil {
		return model.Employee{}, err
	}

	employee, err := s.repo.UpdateEmployee(ctx, employeeID, hrisrepo.UpsertEmployeeParams{
		FullName:          strings.TrimSpace(request.FullName),
		Email:             strings.ToLower(strings.TrimSpace(request.Email)),
		Phone:             trimOptionalString(request.Phone),
		Position:          strings.TrimSpace(request.Position),
		Department:        trimOptionalString(request.Department),
		DateJoined:        dateJoined,
		EmploymentStatus:  strings.TrimSpace(request.EmploymentStatus),
		Address:           trimOptionalString(request.Address),
		EmergencyContact:  trimOptionalString(request.EmergencyContact),
		AvatarURL:         trimOptionalString(request.AvatarURL),
		BankAccountNumber: trimOptionalString(request.BankAccountNumber),
		BankName:          trimOptionalString(request.BankName),
		LinkedInProfile:   trimOptionalString(request.LinkedInProfile),
		SSHKeys:           trimOptionalString(request.SSHKeys),
	})
	if err != nil {
		return model.Employee{}, mapEmployeeError(err)
	}

	if employee.UserID != nil && s.authRepo != nil {
		_ = s.authRepo.UpdateUserFields(ctx, *employee.UserID, employee.FullName, employee.Email)
		_ = s.authRepo.UpdateUserAvatar(ctx, *employee.UserID, employee.AvatarURL)
	}

	return employee, nil
}

func (s *EmployeesService) DeleteEmployee(ctx context.Context, employeeID string) error {
	return mapEmployeeError(s.repo.DeleteEmployee(ctx, employeeID))
}

func (s *EmployeesService) UpdateEmployeeAvatar(ctx context.Context, employeeID string, avatarURL string) (model.Employee, error) {
	value := strings.TrimSpace(avatarURL)
	employee, err := s.repo.UpdateEmployeeAvatar(ctx, employeeID, &value)
	if err != nil {
		return model.Employee{}, mapEmployeeError(err)
	}

	if employee.UserID != nil && s.authRepo != nil {
		_ = s.authRepo.UpdateUserAvatar(ctx, *employee.UserID, employee.AvatarURL)
	}

	return employee, nil
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
