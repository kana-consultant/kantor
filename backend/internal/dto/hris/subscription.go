package hris

import dto "github.com/kana-consultant/kantor/backend/internal/dto"

type CreateSubscriptionRequest struct {
	Name             string       `json:"name" validate:"required,min=2,max=150"`
	Vendor           string       `json:"vendor" validate:"required,min=2,max=150"`
	Description      *string      `json:"description" validate:"omitempty,max=500"`
	CostAmount       int64        `json:"cost_amount" validate:"required,min=0"`
	CostCurrency     string       `json:"cost_currency" validate:"required,len=3"`
	BillingCycle     string       `json:"billing_cycle" validate:"required,oneof=monthly quarterly yearly"`
	StartDate        dto.DateOnly `json:"start_date" validate:"required,datetime=2006-01-02"`
	RenewalDate      dto.DateOnly `json:"renewal_date" validate:"required,datetime=2006-01-02"`
	Status           string       `json:"status" validate:"required,oneof=active cancelled expired"`
	PICEmployeeID    *string      `json:"pic_employee_id" validate:"omitempty,uuid4"`
	Category         string       `json:"category" validate:"required,min=2,max=100"`
	LoginCredentials *string      `json:"login_credentials" validate:"omitempty,max=2000"`
	Notes            *string      `json:"notes" validate:"omitempty,max=2000"`
}

type UpdateSubscriptionRequest = CreateSubscriptionRequest
