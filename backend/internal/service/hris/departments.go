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
	ErrDepartmentNotFound    = errors.New("department not found")
	ErrDepartmentNameExists  = errors.New("department name already exists")
	ErrDepartmentHeadMissing = errors.New("department head employee not found")
)

type DepartmentsService struct {
	repo          *hrisrepo.DepartmentsRepository
	employeesRepo *hrisrepo.EmployeesRepository
}

func NewDepartmentsService(repo *hrisrepo.DepartmentsRepository, employeesRepo *hrisrepo.EmployeesRepository) *DepartmentsService {
	return &DepartmentsService{
		repo:          repo,
		employeesRepo: employeesRepo,
	}
}

func (s *DepartmentsService) CreateDepartment(ctx context.Context, request hrisdto.CreateDepartmentRequest) (model.Department, error) {
	department, err := s.repo.CreateDepartment(ctx, hrisrepo.UpsertDepartmentParams{
		Name:        strings.TrimSpace(request.Name),
		Description: trimOptionalString(request.Description),
		HeadID:      trimOptionalString(request.HeadID),
	})
	if err != nil {
		return model.Department{}, mapDepartmentError(err)
	}

	return department, nil
}

func (s *DepartmentsService) ListDepartments(ctx context.Context) ([]model.Department, error) {
	return s.repo.ListDepartments(ctx)
}

func (s *DepartmentsService) GetDepartment(ctx context.Context, departmentID string) (model.Department, error) {
	department, err := s.repo.GetDepartmentByID(ctx, departmentID)
	if err != nil {
		return model.Department{}, mapDepartmentError(err)
	}

	return department, nil
}

func (s *DepartmentsService) UpdateDepartment(ctx context.Context, departmentID string, request hrisdto.UpdateDepartmentRequest) (model.Department, error) {
	current, err := s.repo.GetDepartmentByID(ctx, departmentID)
	if err != nil {
		return model.Department{}, mapDepartmentError(err)
	}

	updated, err := s.repo.UpdateDepartment(ctx, departmentID, hrisrepo.UpsertDepartmentParams{
		Name:        strings.TrimSpace(request.Name),
		Description: trimOptionalString(request.Description),
		HeadID:      trimOptionalString(request.HeadID),
	})
	if err != nil {
		return model.Department{}, mapDepartmentError(err)
	}

	if current.Name != updated.Name {
		if err := s.employeesRepo.RenameDepartmentReferences(ctx, current.Name, updated.Name); err != nil {
			return model.Department{}, err
		}
	}

	return updated, nil
}

func (s *DepartmentsService) DeleteDepartment(ctx context.Context, departmentID string) error {
	deletedName, err := s.repo.DeleteDepartment(ctx, departmentID)
	if err != nil {
		return mapDepartmentError(err)
	}

	return s.employeesRepo.ClearDepartmentReferences(ctx, deletedName)
}

func mapDepartmentError(err error) error {
	switch {
	case errors.Is(err, hrisrepo.ErrDepartmentNotFound):
		return ErrDepartmentNotFound
	case errors.Is(err, hrisrepo.ErrDepartmentNameExists):
		return ErrDepartmentNameExists
	case errors.Is(err, hrisrepo.ErrDepartmentHeadMissing):
		return ErrDepartmentHeadMissing
	default:
		return err
	}
}

func trimOptionalString(value *string) *string {
	if value == nil {
		return nil
	}

	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}
