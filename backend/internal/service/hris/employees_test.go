package hris

import (
	"context"
	"errors"
	"testing"

	"github.com/kana-consultant/kantor/backend/internal/dto"
	hrisdto "github.com/kana-consultant/kantor/backend/internal/dto/hris"
	"github.com/kana-consultant/kantor/backend/internal/model"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
)

type fakeEmployeesRepo struct {
	employee model.Employee
	updated  *hrisrepo.UpsertEmployeeParams
}

func (r *fakeEmployeesRepo) CreateEmployee(context.Context, hrisrepo.UpsertEmployeeParams) (model.Employee, error) {
	return model.Employee{}, errors.New("not used")
}
func (r *fakeEmployeesRepo) ListEmployees(context.Context, hrisrepo.ListEmployeesParams) ([]model.Employee, int64, error) {
	return nil, 0, nil
}
func (r *fakeEmployeesRepo) GetEmployeeByID(context.Context, string) (model.Employee, error) {
	return r.employee, nil
}
func (r *fakeEmployeesRepo) GetEmployeeByUserID(context.Context, string) (model.Employee, error) {
	return r.employee, nil
}
func (r *fakeEmployeesRepo) UpdateEmployee(_ context.Context, _ string, params hrisrepo.UpsertEmployeeParams) (model.Employee, error) {
	r.updated = &params
	updated := r.employee
	updated.FullName, updated.Email, updated.BankAccountNumber = params.FullName, params.Email, params.BankAccountNumber
	return updated, nil
}
func (r *fakeEmployeesRepo) UpdateEmployeeAvatar(context.Context, string, *string) (model.Employee, error) {
	return r.employee, nil
}
func (r *fakeEmployeesRepo) DeleteEmployee(context.Context, string) error { return nil }

type fakeUserSyncer struct{ names []string }

func (f *fakeUserSyncer) UpdateUserFullName(_ context.Context, _ string, fullName string) error {
	f.names = append(f.names, fullName)
	return nil
}
func (f *fakeUserSyncer) UpdateUserAvatar(context.Context, string, *string) error { return nil }

// The HR form cannot change a linked employee's e-mail: it is the login
// e-mail payslips are sent to, changed only by the user with their
// password. Only the name is copied to the user.
func TestUpdateEmployeeKeepsLoginEmail(t *testing.T) {
	userID := "u-fajar"
	repo := &fakeEmployeesRepo{employee: model.Employee{ID: "e-fajar", UserID: &userID, FullName: "Fajar Nugroho", Email: "fajar.nugroho@kantor.local", BankAccountNumber: strPtr("1234567890")}}
	syncer := &fakeUserSyncer{}
	service := NewEmployeesService(repo)
	service.SetAuthRepo(syncer)
	request := hrisdto.UpdateEmployeeRequest{
		FullName: "Fajar Nugroho", Email: "payroll-collector@evil.test", Position: "Full Time",
		DateJoined: dto.DateOnly("2024-01-02"), EmploymentStatus: "active", BankAccountNumber: strPtr("******7890"),
	}
	if _, err := service.UpdateEmployee(context.Background(), "e-fajar", request); !errors.Is(err, ErrEmployeeLinkedEmailLocked) {
		t.Fatalf("redirecting a linked employee's e-mail err = %v", err)
	}
	if repo.updated != nil {
		t.Fatal("nothing may be written")
	}

	request.Email = "Fajar.Nugroho@kantor.local"
	request.FullName = "Fajar N."
	updated, err := service.UpdateEmployee(context.Background(), "e-fajar", request)
	if err != nil {
		t.Fatal(err)
	}
	if *updated.BankAccountNumber != "1234567890" {
		t.Fatalf("masked bank number overwrote the stored one: %v", *updated.BankAccountNumber)
	}
	if len(syncer.names) != 1 || syncer.names[0] != "Fajar N." {
		t.Fatalf("synced names = %v", syncer.names)
	}

	// An unlinked employee's e-mail is HR data and may change.
	repo.employee.UserID = nil
	request.Email = "fajar.baru@kantor.local"
	if _, err := service.UpdateEmployee(context.Background(), "e-fajar", request); err != nil {
		t.Fatalf("unlinked e-mail change: %v", err)
	}
}
