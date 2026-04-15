package hris

import (
	"context"
	"testing"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/model"
	"github.com/kana-consultant/kantor/backend/internal/rbac"
	"github.com/kana-consultant/kantor/backend/internal/security"
)

type overviewRepoStub struct {
	overview              model.HrisOverview
	overviewErr           error
	payrollCiphertexts    []string
	payrollCiphertextsErr error
	payrollHistory        []model.HrisOverviewSalaryHistoryRow
	payrollHistoryErr     error
	subscriptions         []model.HrisOverviewSubscriptionRow
	subscriptionsErr      error
	gotEmployeeFilter     string
}

func (s *overviewRepoStub) GetOverview(_ context.Context, _ time.Time, employeeFilter string) (model.HrisOverview, error) {
	s.gotEmployeeFilter = employeeFilter
	return s.overview, s.overviewErr
}

func (s *overviewRepoStub) ListLatestActivePayrollCiphertexts(context.Context) ([]string, error) {
	return s.payrollCiphertexts, s.payrollCiphertextsErr
}

func (s *overviewRepoStub) ListActivePayrollHistoryRows(context.Context) ([]model.HrisOverviewSalaryHistoryRow, error) {
	return s.payrollHistory, s.payrollHistoryErr
}

func (s *overviewRepoStub) ListActiveSubscriptionsForOverview(context.Context) ([]model.HrisOverviewSubscriptionRow, error) {
	return s.subscriptions, s.subscriptionsErr
}

type overviewEmployeesRepoStub struct {
	employee model.Employee
	err      error
}

func (s *overviewEmployeesRepoStub) GetEmployeeByUserID(context.Context, string) (model.Employee, error) {
	return s.employee, s.err
}

func TestOverviewServiceTotalMonthlyPayroll(t *testing.T) {
	t.Parallel()

	encrypter, err := security.NewEncrypter("test-secret")
	if err != nil {
		t.Fatalf("NewEncrypter returned error: %v", err)
	}

	first, err := encrypter.EncryptString("5000000")
	if err != nil {
		t.Fatalf("EncryptString returned error: %v", err)
	}
	second, err := encrypter.EncryptString("7250000")
	if err != nil {
		t.Fatalf("EncryptString returned error: %v", err)
	}

	service := NewOverviewService(&overviewRepoStub{
		payrollCiphertexts: []string{first, second},
	}, &overviewEmployeesRepoStub{}, encrypter)

	total, err := service.totalMonthlyPayroll(context.Background())
	if err != nil {
		t.Fatalf("totalMonthlyPayroll returned error: %v", err)
	}
	if total != 12250000 {
		t.Fatalf("totalMonthlyPayroll() = %d, want %d", total, 12250000)
	}
}

func TestOverviewServiceGetOverviewScopesEmployeeWhenNotViewAll(t *testing.T) {
	t.Parallel()

	encrypter, err := security.NewEncrypter("test-secret")
	if err != nil {
		t.Fatalf("NewEncrypter returned error: %v", err)
	}

	ciphertext, err := encrypter.EncryptString("5000000")
	if err != nil {
		t.Fatalf("EncryptString returned error: %v", err)
	}

	repo := &overviewRepoStub{
		overview:           model.HrisOverview{TotalEmployees: 4},
		payrollCiphertexts: []string{ciphertext},
		payrollHistory: []model.HrisOverviewSalaryHistoryRow{
			{
				EmployeeID:         "employee-123",
				EffectiveDate:      time.Now().AddDate(0, -1, 0),
				NetSalaryEncrypted: ciphertext,
			},
		},
	}
	employeesRepo := &overviewEmployeesRepoStub{
		employee: model.Employee{ID: "employee-123"},
	}
	service := NewOverviewService(repo, employeesRepo, encrypter)

	overview, err := service.GetOverview(context.Background(), "user-123", &rbac.CachedPermissions{
		Permissions: map[string]bool{},
	})
	if err != nil {
		t.Fatalf("GetOverview returned error: %v", err)
	}

	if repo.gotEmployeeFilter != "employee-123" {
		t.Fatalf("GetOverview employee filter = %q, want %q", repo.gotEmployeeFilter, "employee-123")
	}
	if overview.TotalMonthlyPayroll != 5000000 {
		t.Fatalf("GetOverview TotalMonthlyPayroll = %d, want %d", overview.TotalMonthlyPayroll, 5000000)
	}
}

func TestOverviewServiceGetOverviewAddsRecurringPayrollAndSubscriptionToSeries(t *testing.T) {
	t.Parallel()

	encrypter, err := security.NewEncrypter("test-secret")
	if err != nil {
		t.Fatalf("NewEncrypter returned error: %v", err)
	}

	ciphertext, err := encrypter.EncryptString("5000000")
	if err != nil {
		t.Fatalf("EncryptString returned error: %v", err)
	}

	now := time.Now()
	currentKey := now.Format("2006-01")
	repo := &overviewRepoStub{
		overview: model.HrisOverview{
			IncomeVsOutcome: []model.FinanceOverviewPoint{
				{
					Key:     currentKey,
					Label:   now.Format("Jan"),
					Income:  12000000,
					Outcome: 5000000,
				},
			},
		},
		payrollCiphertexts: []string{ciphertext},
		payrollHistory: []model.HrisOverviewSalaryHistoryRow{
			{
				EmployeeID:         "employee-1",
				EffectiveDate:      now.AddDate(0, -3, 0),
				NetSalaryEncrypted: ciphertext,
			},
		},
		subscriptions: []model.HrisOverviewSubscriptionRow{
			{
				StartDate:    now.AddDate(0, -2, 0),
				BillingCycle: "monthly",
				CostAmount:   1500000,
			},
		},
	}
	service := NewOverviewService(repo, &overviewEmployeesRepoStub{}, encrypter)

	overview, err := service.GetOverview(context.Background(), "user-123", &rbac.CachedPermissions{
		Permissions: map[string]bool{
			"hris:reimbursement:view_all": true,
		},
	})
	if err != nil {
		t.Fatalf("GetOverview returned error: %v", err)
	}

	if overview.IncomeVsOutcome[0].Outcome != 11500000 {
		t.Fatalf("GetOverview outcome = %d, want %d", overview.IncomeVsOutcome[0].Outcome, 11500000)
	}
	if overview.MonthlyNet != 500000 {
		t.Fatalf("GetOverview monthly net = %d, want %d", overview.MonthlyNet, 500000)
	}
}
