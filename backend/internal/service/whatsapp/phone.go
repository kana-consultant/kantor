package whatsapp

import "strings"

// NormalizePhone converts Indonesian phone numbers to the 628xxx format.
// Accepted inputs: 08xxx, +628xxx, 628xxx.
func NormalizePhone(phone string) string {
	phone = strings.TrimSpace(phone)
	phone = strings.ReplaceAll(phone, " ", "")
	phone = strings.ReplaceAll(phone, "-", "")

	if strings.HasPrefix(phone, "+") {
		phone = phone[1:]
	}

	if strings.HasPrefix(phone, "08") {
		phone = "62" + phone[1:]
	}

	return phone
}

// PhoneToChatID converts a normalized phone number to a WAHA chat ID.
func PhoneToChatID(phone string) string {
	return NormalizePhone(phone) + "@c.us"
}
