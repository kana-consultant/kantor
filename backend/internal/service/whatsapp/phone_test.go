package whatsapp

import "testing"

func TestNormalizePhone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{input: "08123456789", want: "628123456789"},
		{input: "8123456789", want: "628123456789"},
		{input: "+62 812-3456-789", want: "628123456789"},
		{input: "628123456789", want: "628123456789"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			if got := NormalizePhone(tt.input); got != tt.want {
				t.Fatalf("NormalizePhone(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsValidPhone(t *testing.T) {
	t.Parallel()

	if !IsValidPhone("628123456789") {
		t.Fatal("expected normalized Indonesian phone to be valid")
	}
	if IsValidPhone("08123456789") {
		t.Fatal("expected non-normalized phone to be invalid")
	}
	if IsValidPhone("62812abc6789") {
		t.Fatal("expected non-digit phone to be invalid")
	}
}

func TestPhoneToChatID(t *testing.T) {
	t.Parallel()

	if got := PhoneToChatID("08123456789"); got != "628123456789@c.us" {
		t.Fatalf("PhoneToChatID() = %q, want %q", got, "628123456789@c.us")
	}
}
