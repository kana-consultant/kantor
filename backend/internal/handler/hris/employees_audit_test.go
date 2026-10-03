package hris

import (
	"reflect"
	"testing"
	"time"

	hrisdto "github.com/kana-consultant/kantor/backend/internal/dto/hris"
	"github.com/kana-consultant/kantor/backend/internal/model"
)

func TestChangedEmployeeFieldsNamesBankChanges(t *testing.T) {
	str := func(value string) *string { return &value }
	joined := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	previous := model.Employee{
		FullName: "Budi", Email: "budi@example.test", Position: "Engineer",
		DateJoined: joined, EmploymentStatus: "active",
		BankAccountNumber: str("1234567890"), BankName: str("BCA"),
	}

	// An unchanged save (masked number substituted by the service) lists
	// nothing.
	if fields := changedEmployeeFields(previous, previous); len(fields) != 0 {
		t.Errorf("no-op save = %v", fields)
	}

	next := previous
	next.BankAccountNumber = str("7777000099")
	next.Phone = str("0812")
	if fields := changedEmployeeFields(previous, next); !reflect.DeepEqual(fields, []string{"phone", "bank_account_number"}) {
		t.Errorf("changed = %v", fields)
	}

	value := withChangedFields(hrisdto.UpdateEmployeeRequest{FullName: "Budi"}, []string{"bank_account_number"})
	object, ok := value.(map[string]any)
	if !ok || object["full_name"] != "Budi" || !reflect.DeepEqual(object["changed_fields"], []string{"bank_account_number"}) {
		t.Errorf("audit value = %#v", value)
	}
}
