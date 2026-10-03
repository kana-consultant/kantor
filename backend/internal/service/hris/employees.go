package hris

import (
	"context"
	"errors"
	"strings"

	"github.com/kana-consultant/kantor/backend/internal/docgen"
	shareddto "github.com/kana-consultant/kantor/backend/internal/dto"
	hrisdto "github.com/kana-consultant/kantor/backend/internal/dto/hris"
	"github.com/kana-consultant/kantor/backend/internal/model"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
)

var (
	ErrEmployeeNotFound        = errors.New("employee not found")
	ErrEmployeeEmailExists     = errors.New("employee email already exists")
	ErrEmployeeUserLinkedTwice = errors.New("user account is already linked to another employee")
	// ErrEmployeeHasDocuments: payslips (and contracts) reference the
	// employee with ON DELETE RESTRICT; they are legal records.
	ErrEmployeeHasDocuments = errors.New("karyawan memiliki slip gaji atau kontrak kerja yang harus disimpan; ubah status karyawan menjadi resigned/terminated alih-alih menghapus")
	// ErrEmployeeLinkedEmailLocked: a linked employee's e-mail is the
	// user's login e-mail, where payslips and contracts are sent; only the
	// user changes it (with their password).
	ErrEmployeeLinkedEmailLocked = errors.New("email karyawan ini adalah email login akunnya dan hanya dapat diubah oleh pemilik akun melalui Profil (dengan kata sandi)")
)

// userFieldsSyncer copies the HR form's name and avatar to the linked user.
// The e-mail is never copied: users.email is the login and the address
// documents are sent to, changed only by the user with their password.
type userFieldsSyncer interface {
	UpdateUserFullName(ctx context.Context, userID string, fullName string) error
	UpdateUserAvatar(ctx context.Context, userID string, avatarURL *string) error
}

type employeesRepository interface {
	CreateEmployee(ctx context.Context, params hrisrepo.UpsertEmployeeParams) (model.Employee, error)
	ListEmployees(ctx context.Context, params hrisrepo.ListEmployeesParams) ([]model.Employee, int64, error)
	GetEmployeeByID(ctx context.Context, employeeID string) (model.Employee, error)
	GetEmployeeByUserID(ctx context.Context, userID string) (model.Employee, error)
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

func (s *EmployeesService) GetMyEmployee(ctx context.Context, userID string) (model.Employee, error) {
	employee, err := s.repo.GetEmployeeByUserID(ctx, userID)
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

	current, err := s.repo.GetEmployeeByID(ctx, employeeID)
	if err != nil {
		return model.Employee{}, mapEmployeeError(err)
	}
	// A linked employee's e-mail mirrors the login e-mail (documents go
	// there), so hris:employee:edit must not redirect it.
	if current.UserID != nil && !strings.EqualFold(strings.TrimSpace(request.Email), strings.TrimSpace(current.Email)) {
		return model.Employee{}, ErrEmployeeLinkedEmailLocked
	}
	// The HR form is a full replacement and callers without
	// hris:employee_identity:view only ever saw the masked account number:
	// a submitted mask keeps the stored number instead of overwriting it.
	if IsMaskedBankAccount(request.BankAccountNumber) {
		request.BankAccountNumber = current.BankAccountNumber
	}
	// A stored number that cannot be decrypted reads as empty; a save
	// without a new number keeps it instead of discarding it.
	keepStoredBankAccount := current.BankAccountUnreadable &&
		(request.BankAccountNumber == nil || strings.TrimSpace(*request.BankAccountNumber) == "")

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

		KeepStoredBankAccount: keepStoredBankAccount,
	})
	if err != nil {
		return model.Employee{}, mapEmployeeError(err)
	}

	if employee.UserID != nil && s.authRepo != nil {
		_ = s.authRepo.UpdateUserFullName(ctx, *employee.UserID, employee.FullName)
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

// MaskEmployeeBankAccount returns e with the bank account number masked
// ('******7890'), for callers who may not see it in full.
func MaskEmployeeBankAccount(e model.Employee) model.Employee {
	if e.BankAccountNumber != nil && strings.TrimSpace(*e.BankAccountNumber) != "" {
		masked := docgen.MaskAccount(*e.BankAccountNumber)
		e.BankAccountNumber = &masked
	}
	return e
}

// IsMaskedBankAccount reports whether a submitted account number is a mask
// echoed back by the client ('******7890'). A real account number never
// contains '*'.
func IsMaskedBankAccount(value *string) bool {
	return value != nil && strings.Contains(*value, "*")
}

func mapEmployeeError(err error) error {
	switch {
	case errors.Is(err, hrisrepo.ErrEmployeeHasDocuments):
		return ErrEmployeeHasDocuments
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
