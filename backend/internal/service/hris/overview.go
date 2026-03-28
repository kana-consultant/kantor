package hris

import (
	"context"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/model"
	"github.com/kana-consultant/kantor/backend/internal/rbac"
)

type hrisOverviewRepository interface {
	GetOverview(ctx context.Context, now time.Time, employeeFilter string) (model.HrisOverview, error)
}

type overviewEmployeesRepository interface {
	GetEmployeeByUserID(ctx context.Context, userID string) (model.Employee, error)
}

type OverviewService struct {
	repo          hrisOverviewRepository
	employeesRepo overviewEmployeesRepository
}

func NewOverviewService(repo hrisOverviewRepository, employeesRepo overviewEmployeesRepository) *OverviewService {
	return &OverviewService{repo: repo, employeesRepo: employeesRepo}
}

func (s *OverviewService) GetOverview(ctx context.Context, actorID string, perms *rbac.CachedPermissions) (model.HrisOverview, error) {
	employeeFilter := ""
	if !rbac.CanViewAll(perms, "hris:reimbursement:view_all") {
		employee, err := s.employeesRepo.GetEmployeeByUserID(ctx, actorID)
		if err == nil {
			employeeFilter = employee.ID
		}
		// if the user has no employee record, employeeFilter stays "" — they'll see nothing in reimbursements
	}
	return s.repo.GetOverview(ctx, time.Now(), employeeFilter)
}
