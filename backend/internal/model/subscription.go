package model

import "time"

type Subscription struct {
	ID                    string    `json:"id"`
	Name                  string    `json:"name"`
	Vendor                string    `json:"vendor"`
	Description           *string   `json:"description,omitempty"`
	CostAmount            int64     `json:"cost_amount"`
	CostCurrency          string    `json:"cost_currency"`
	BillingCycle          string    `json:"billing_cycle"`
	StartDate             time.Time `json:"start_date"`
	RenewalDate           time.Time `json:"renewal_date"`
	Status                string    `json:"status"`
	PICEmployeeID         *string   `json:"pic_employee_id,omitempty"`
	PICEmployeeName       *string   `json:"pic_employee_name,omitempty"`
	Category              string    `json:"category"`
	LoginCredentialsPlain *string   `json:"login_credentials,omitempty"`
	Notes                 *string   `json:"notes,omitempty"`
	CreatedBy             string    `json:"created_by"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type SubscriptionAlert struct {
	ID               string    `json:"id"`
	SubscriptionID   string    `json:"subscription_id"`
	SubscriptionName string    `json:"subscription_name"`
	AlertType        string    `json:"alert_type"`
	IsRead           bool      `json:"is_read"`
	CreatedAt        time.Time `json:"created_at"`
}

type SubscriptionSummary struct {
	TotalMonthlyCost int64            `json:"total_monthly_cost"`
	TotalYearlyCost  int64            `json:"total_yearly_cost"`
	ActiveCount      int64            `json:"active_count"`
	ByCategory       map[string]int64 `json:"by_category"`
}
