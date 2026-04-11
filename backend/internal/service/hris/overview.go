package hris

import (
	"context"
	"strconv"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/model"
	"github.com/kana-consultant/kantor/backend/internal/rbac"
	"github.com/kana-consultant/kantor/backend/internal/security"
)

type hrisOverviewRepository interface {
	GetOverview(ctx context.Context, now time.Time, employeeFilter string) (model.HrisOverview, error)
	ListLatestActivePayrollCiphertexts(ctx context.Context) ([]string, error)
}

type overviewEmployeesRepository interface {
	GetEmployeeByUserID(ctx context.Context, userID string) (model.Employee, error)
}

type OverviewService struct {
	repo          hrisOverviewRepository
	employeesRepo overviewEmployeesRepository
	encrypter     *security.Encrypter
}

func NewOverviewService(repo hrisOverviewRepository, employeesRepo overviewEmployeesRepository, encrypter *security.Encrypter) *OverviewService {
	return &OverviewService{repo: repo, employeesRepo: employeesRepo, encrypter: encrypter}
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
	overview, err := s.repo.GetOverview(ctx, time.Now(), employeeFilter)
	if err != nil {
		return model.HrisOverview{}, err
	}

	totalPayroll, err := s.totalMonthlyPayroll(ctx)
	if err != nil {
		return model.HrisOverview{}, err
	}
	overview.TotalMonthlyPayroll = totalPayroll

	return overview, nil
}

func (s *OverviewService) totalMonthlyPayroll(ctx context.Context) (int64, error) {
	ciphertexts, err := s.repo.ListLatestActivePayrollCiphertexts(ctx)
	if err != nil {
		return 0, err
	}

	var total int64
	for _, ciphertext := range ciphertexts {
		plaintext, err := s.encrypter.DecryptString(ciphertext)
		if err != nil {
			return 0, err
		}
		value, err := strconv.ParseInt(plaintext, 10, 64)
		if err != nil {
			return 0, err
		}
		total += value
	}

	return total, nil
}
