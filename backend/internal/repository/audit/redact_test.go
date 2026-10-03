package audit

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

func decodeAudit(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return out
}

func TestRedactJSONNestedArraysAndCase(t *testing.T) {
	raw := []byte(`{
		"full_name": "Budi",
		"Bank_Account_Number": "1234567890",
		"BankAccountNumber": "1234567890",
		"bank-account-number": "1234567890",
		"NIK": "3273015402980001",
		"identity": {"nik": "3273015402980001", "birth_place": "Bandung"},
		"salary": {"base_salary": 15000000, "allowances": {"transport": 500000}, "deductions": [{"bpjs": 100000}], "effective_date": "2026-09-01"},
		"items": [{"amount": 750000, "reason": "Bonus"}, {"note": "x", "nested": [{"Net_Salary": 1}]}],
		"settings": {"smtp_password": "abcd efgh ijkl mnop", "API_KEY": "sk-123", "password": "p", "app_password": "q"},
		"bank_account_encrypted": "v1:xyz",
		"bank_name": "BCA",
		"amount_note": "kept",
		"cleared": {"bank_account_number": null}
	}`)
	redacted, changed, err := RedactJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected a change")
	}
	text := string(redacted)
	for _, secret := range []string{"1234567890", "3273015402980001", "15000000", "500000", "100000", "750000", "abcd efgh", "sk-123", "v1:xyz"} {
		if strings.Contains(text, secret) {
			t.Errorf("redacted JSON still contains %q: %s", secret, text)
		}
	}

	out := decodeAudit(t, redacted)
	for _, key := range []string{"Bank_Account_Number", "BankAccountNumber", "bank-account-number", "NIK", "identity", "bank_account_encrypted"} {
		if out[key] != RedactedValue {
			t.Errorf("%s = %v, want %q", key, out[key], RedactedValue)
		}
	}
	// Keys stay, non-denylisted values stay.
	if out["full_name"] != "Budi" || out["bank_name"] != "BCA" || out["amount_note"] != "kept" {
		t.Errorf("kept values changed: %v", out)
	}
	salary := out["salary"].(map[string]any)
	if salary["base_salary"] != RedactedValue || salary["allowances"] != RedactedValue || salary["deductions"] != RedactedValue || salary["effective_date"] != "2026-09-01" {
		t.Errorf("salary = %v", salary)
	}
	items := out["items"].([]any)
	first := items[0].(map[string]any)
	if first["amount"] != RedactedValue || first["reason"] != "Bonus" {
		t.Errorf("items[0] = %v", first)
	}
	nested := items[1].(map[string]any)["nested"].([]any)[0].(map[string]any)
	if nested["Net_Salary"] != RedactedValue {
		t.Errorf("nested net salary = %v", nested)
	}
	settings := out["settings"].(map[string]any)
	for _, key := range []string{"smtp_password", "API_KEY", "password", "app_password"} {
		if settings[key] != RedactedValue {
			t.Errorf("settings.%s = %v", key, settings[key])
		}
	}
	// null only says "empty" and stays null.
	if cleared := out["cleared"].(map[string]any); cleared["bank_account_number"] != nil {
		t.Errorf("null value = %v", cleared["bank_account_number"])
	}
}

func TestRedactJSONUnchangedAndIdempotent(t *testing.T) {
	raw := []byte(`{"employee_id":"e1","records_count":3,"big":12345678901234567890}`)
	redacted, changed, err := RedactJSON(raw)
	if err != nil || changed || string(redacted) != string(raw) {
		t.Fatalf("unchanged input = %s, %v, %v", redacted, changed, err)
	}

	once, changed, err := RedactJSON([]byte(`[{"amount": 1}, "amount", 2]`))
	if err != nil || !changed {
		t.Fatalf("array input = %s, %v, %v", once, changed, err)
	}
	if string(once) != `[{"amount":"[redacted]"},"amount",2]` {
		t.Errorf("array redaction = %s", once)
	}
	twice, changed, err := RedactJSON(once)
	if err != nil || changed || string(twice) != string(once) {
		t.Errorf("second pass = %s, %v, %v", twice, changed, err)
	}

	for _, input := range []string{"", "null", `"text"`, "42"} {
		if out, changed, err := RedactJSON([]byte(input)); err != nil || changed || string(out) != input {
			t.Errorf("%q -> %q, %v, %v", input, out, changed, err)
		}
	}
	if _, _, err := RedactJSON([]byte(`{"broken"`)); err == nil {
		t.Error("invalid JSON must error")
	}
}

// The insert path redacts whatever a handler passes: the salary, bonus and
// employee request DTOs, structs without json tags, and maps.
func TestMarshalNullableJSONRedactsCallerValues(t *testing.T) {
	type salaryRequest struct {
		BaseSalary    int64            `json:"base_salary"`
		Allowances    map[string]int64 `json:"allowances"`
		Deductions    map[string]int64 `json:"deductions"`
		EffectiveDate string           `json:"effective_date"`
	}
	type untagged struct {
		BankAccountNumber *string
		Amount            int64
		Reason            string
	}
	account := "9876543210"
	cases := map[string]struct {
		value any
		keys  []string
		keep  map[string]any
	}{
		"salary": {
			value: salaryRequest{BaseSalary: 15_000_000, Allowances: map[string]int64{"meal": 1}, Deductions: map[string]int64{"bpjs": 2}, EffectiveDate: "2026-09-01"},
			keys:  []string{"base_salary", "allowances", "deductions"},
			keep:  map[string]any{"effective_date": "2026-09-01"},
		},
		"untagged struct": {
			value: untagged{BankAccountNumber: &account, Amount: 5, Reason: "THR"},
			keys:  []string{"BankAccountNumber", "Amount"},
			keep:  map[string]any{"Reason": "THR"},
		},
		"map": {
			value: map[string]any{"smtp_username": "slip@example.com", "smtp_password": "secret"},
			keys:  []string{"smtp_password"},
			keep:  map[string]any{"smtp_username": "slip@example.com"},
		},
	}
	for name, tc := range cases {
		raw, err := marshalNullableJSON(tc.value)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		out := decodeAudit(t, raw)
		for _, key := range tc.keys {
			if out[key] != RedactedValue {
				t.Errorf("%s: %s = %v", name, key, out[key])
			}
		}
		for key, want := range tc.keep {
			if out[key] != want {
				t.Errorf("%s: %s = %v, want %v", name, key, out[key], want)
			}
		}
	}
	if raw, err := marshalNullableJSON(nil); err != nil || raw != nil {
		t.Errorf("nil value = %s, %v", raw, err)
	}
}

func TestIsRedactedKey(t *testing.T) {
	for _, key := range []string{"bank_account_number", "BANK_ACCOUNT_NUMBER", "bankAccountNumber", "nik", "Identity", "base_salary", "allowances", "deductions", "amount", "net_salary", "smtp_password", "api_key", "apiKey", "password", "app_password", "bank_account_encrypted"} {
		if !IsRedactedKey(key) {
			t.Errorf("%q must be redacted", key)
		}
	}
	for _, key := range []string{"bank_name", "amount_note", "employee_id", "identity_updated_at", "password_changed", "full_name", "changed_fields"} {
		if IsRedactedKey(key) {
			t.Errorf("%q must not be redacted", key)
		}
	}
}

// The scrub pre-filter must match every spelling RedactJSON redacts (it may
// match more; RedactJSON decides).
func TestRedactedKeyPatternCoversDenylist(t *testing.T) {
	pattern := regexp.MustCompile(`(?i)` + redactedKeyPattern())
	for _, text := range []string{
		`{"bank_account_number": "1"}`,
		`{"BankAccountNumber":"1"}`,
		`{"x": [{"Amount" : 1}]}`,
		`{"smtp-password": "x"}`,
		`{"NIK": "x"}`,
	} {
		if !pattern.MatchString(text) {
			t.Errorf("pattern misses %s", text)
		}
	}
	for _, text := range []string{`{"bank_name": "BCA"}`, `{"note": "amount"}`, `{"total_amount": 1}`} {
		if pattern.MatchString(text) {
			t.Errorf("pattern matches %s", text)
		}
	}
}
