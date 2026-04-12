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
	gotEmployeeFilter     string
}

func (s *overviewRepoStub) GetOverview(_ context.Context, _ time.Time, employeeFilter string) (model.HrisOverview, error) {
	s.gotEmployeeFilter = employeeFilter
	return s.overview, s.overviewErr
}

func (s *overviewRepoStub) ListLatestActivePayrollCiphertexts(context.Context) ([]string, error) {
	return s.payrollCiphertexts, s.payrollCiphertextsErr
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
