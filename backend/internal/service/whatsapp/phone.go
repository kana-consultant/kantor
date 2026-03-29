package whatsapp

import "strings"

// NormalizePhone converts Indonesian phone numbers to the 628xxx format.
// Accepted inputs: 08xxx, 8xxx, +628xxx, 628xxx.
func NormalizePhone(phone string) string {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return ""
	}

	if strings.HasPrefix(phone, "+") {
		phone = phone[1:]
	}

	phone = strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, phone)

	if strings.HasPrefix(phone, "08") {
		phone = "62" + phone[1:]
	}
	if strings.HasPrefix(phone, "8") {
		phone = "62" + phone
	}

	return phone
}

func IsValidPhone(phone string) bool {
	if !strings.HasPrefix(phone, "628") {
		return false
	}
	if len(phone) < 10 || len(phone) > 16 {
		return false
	}
	for _, r := range phone {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// PhoneToChatID converts a normalized phone number to a WAHA chat ID.
func PhoneToChatID(phone string) string {
	return NormalizePhone(phone) + "@c.us"
}
