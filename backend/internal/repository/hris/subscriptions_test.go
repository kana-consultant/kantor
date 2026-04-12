package hris

import "testing"

func TestMonthlyCost(t *testing.T) {
	t.Parallel()

	if got := monthlyCost(120000, "monthly"); got != 120000 {
		t.Fatalf("monthlyCost(monthly) = %d, want 120000", got)
	}
	if got := monthlyCost(120000, "quarterly"); got != 40000 {
		t.Fatalf("monthlyCost(quarterly) = %d, want 40000", got)
	}
	if got := monthlyCost(120000, "yearly"); got != 10000 {
		t.Fatalf("monthlyCost(yearly) = %d, want 10000", got)
	}
}

func TestYearlyCost(t *testing.T) {
	t.Parallel()

	if got := yearlyCost(10000, "monthly"); got != 120000 {
		t.Fatalf("yearlyCost(monthly) = %d, want 120000", got)
	}
	if got := yearlyCost(30000, "quarterly"); got != 120000 {
		t.Fatalf("yearlyCost(quarterly) = %d, want 120000", got)
	}
	if got := yearlyCost(120000, "yearly"); got != 120000 {
		t.Fatalf("yearlyCost(yearly) = %d, want 120000", got)
	}
}
