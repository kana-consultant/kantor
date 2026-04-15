package hris

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/model"
	"github.com/kana-consultant/kantor/backend/internal/rbac"
	"github.com/kana-consultant/kantor/backend/internal/security"
)

type hrisOverviewRepository interface {
	GetOverview(ctx context.Context, now time.Time, employeeFilter string) (model.HrisOverview, error)
	ListLatestActivePayrollCiphertexts(ctx context.Context) ([]string, error)
	ListActivePayrollHistoryRows(ctx context.Context) ([]model.HrisOverviewSalaryHistoryRow, error)
	ListActiveSubscriptionsForOverview(ctx context.Context) ([]model.HrisOverviewSubscriptionRow, error)
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
	payrollHistory, err := s.repo.ListActivePayrollHistoryRows(ctx)
	if err != nil {
		return model.HrisOverview{}, err
	}
	subscriptions, err := s.repo.ListActiveSubscriptionsForOverview(ctx)
	if err != nil {
		return model.HrisOverview{}, err
	}
	if err := s.applyRecurringOutcomes(&overview, time.Now(), payrollHistory, subscriptions); err != nil {
		return model.HrisOverview{}, err
	}

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

func (s *OverviewService) applyRecurringOutcomes(overview *model.HrisOverview, now time.Time, payrollHistory []model.HrisOverviewSalaryHistoryRow, subscriptions []model.HrisOverviewSubscriptionRow) error {
	if overview == nil {
		return nil
	}

	currentMonthKey := now.Format("2006-01")
	for index := range overview.IncomeVsOutcome {
		monthStart, err := time.ParseInLocation("2006-01", overview.IncomeVsOutcome[index].Key, now.Location())
		if err != nil {
			return fmt.Errorf("parse overview month %q: %w", overview.IncomeVsOutcome[index].Key, err)
		}
		monthEnd := monthStart.AddDate(0, 1, 0)

		payrollForMonth, err := s.payrollForMonth(monthEnd, payrollHistory)
		if err != nil {
			return err
		}
		subscriptionForMonth := subscriptionCostForMonth(monthEnd, subscriptions)

		overview.IncomeVsOutcome[index].Outcome += payrollForMonth + subscriptionForMonth
		if overview.IncomeVsOutcome[index].Key == currentMonthKey {
			overview.MonthlyNet = overview.IncomeVsOutcome[index].Income - overview.IncomeVsOutcome[index].Outcome
		}
	}
	return nil
}

func (s *OverviewService) payrollForMonth(monthEnd time.Time, rows []model.HrisOverviewSalaryHistoryRow) (int64, error) {
	if len(rows) == 0 {
		return 0, nil
	}

	latestByEmployee := make(map[string]string, len(rows))
	for _, row := range rows {
		if !row.EffectiveDate.Before(monthEnd) {
			continue
		}
		latestByEmployee[row.EmployeeID] = row.NetSalaryEncrypted
	}

	var total int64
	for _, ciphertext := range latestByEmployee {
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

func subscriptionCostForMonth(monthEnd time.Time, rows []model.HrisOverviewSubscriptionRow) int64 {
	var total int64
	for _, row := range rows {
		if !row.StartDate.Before(monthEnd) {
			continue
		}
		total += monthlySubscriptionCost(row)
	}
	return total
}

func monthlySubscriptionCost(row model.HrisOverviewSubscriptionRow) int64 {
	switch strings.TrimSpace(row.BillingCycle) {
	case "quarterly":
		return row.CostAmount / 3
	case "yearly":
		return row.CostAmount / 12
	default:
		return row.CostAmount
	}
}
