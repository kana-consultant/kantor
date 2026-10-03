package audit

import (
	"bytes"
	"encoding/json"
	"strings"
)

// RedactedValue replaces the value of every denylisted key in audit rows.
const RedactedValue = "[redacted]"

// redactedKeys is the audit denylist: values under these keys never reach
// audit_logs, at any depth. Keys are compared after normalizeAuditKey, so
// "bank_account_number", "BankAccountNumber" and "bank-account-number" are
// the same key. Keep it in sync with docs/hris-documents.md.
var redactedKeys = map[string]struct{}{
	normalizeAuditKey("bank_account_number"):    {},
	normalizeAuditKey("bank_account_encrypted"): {},
	normalizeAuditKey("nik"):                    {},
	normalizeAuditKey("identity"):               {},
	normalizeAuditKey("base_salary"):            {},
	normalizeAuditKey("allowances"):             {},
	normalizeAuditKey("deductions"):             {},
	normalizeAuditKey("amount"):                 {},
	normalizeAuditKey("net_salary"):             {},
	normalizeAuditKey("smtp_password"):          {},
	normalizeAuditKey("api_key"):                {},
	normalizeAuditKey("password"):               {},
	normalizeAuditKey("app_password"):           {},
}

// normalizeAuditKey lower-cases a key and drops '_', '-' and spaces.
func normalizeAuditKey(key string) string {
	var b strings.Builder
	b.Grow(len(key))
	for _, r := range strings.ToLower(key) {
		if r == '_' || r == '-' || r == ' ' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// IsRedactedKey reports whether values under key are kept out of audit rows.
func IsRedactedKey(key string) bool {
	_, ok := redactedKeys[normalizeAuditKey(key)]
	return ok
}

// RedactJSON returns raw with the value of every denylisted object key
// (recursively, through nested objects and arrays) replaced by
// RedactedValue; keys are kept so the row still shows which fields were
// involved. A JSON null stays null (it only says the field was empty).
// changed reports whether anything was replaced; when it is false raw is
// returned as is. Numbers keep their exact text.
func RedactJSON(raw []byte) (redacted []byte, changed bool, err error) {
	return redactJSONWith(raw, nil)
}

// redactJSONWith is RedactJSON that also redacts extraKeys (compared like
// the denylist), for keys that are only secret on some rows.
func redactJSONWith(raw []byte, extraKeys []string) (redacted []byte, changed bool, err error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return raw, false, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, false, err
	}
	var extra map[string]struct{}
	if len(extraKeys) > 0 {
		extra = make(map[string]struct{}, len(extraKeys))
		for _, key := range extraKeys {
			extra[normalizeAuditKey(key)] = struct{}{}
		}
	}
	value, changed = redactValue(value, extra)
	if !changed {
		return raw, false, nil
	}
	redacted, err = json.Marshal(value)
	if err != nil {
		return nil, false, err
	}
	return redacted, true, nil
}

func redactValue(value any, extra map[string]struct{}) (any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		changed := false
		for key, item := range typed {
			_, extraKey := extra[normalizeAuditKey(key)]
			if extraKey || IsRedactedKey(key) {
				if item != nil && item != RedactedValue {
					typed[key] = RedactedValue
					changed = true
				}
				continue
			}
			if next, itemChanged := redactValue(item, extra); itemChanged {
				typed[key] = next
				changed = true
			}
		}
		return typed, changed
	case []any:
		changed := false
		for index, item := range typed {
			if next, itemChanged := redactValue(item, extra); itemChanged {
				typed[index] = next
				changed = true
			}
		}
		return typed, changed
	default:
		return value, false
	}
}
