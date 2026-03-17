package model

import "time"

type FinanceCategory struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	IsDefault bool      `json:"is_default"`
	CreatedAt time.Time `json:"created_at"`
}

type FinanceRecord struct {
	ID             string     `json:"id"`
	CategoryID     string     `json:"category_id"`
	CategoryName   string     `json:"category_name"`
	Type           string     `json:"type"`
	Amount         int64      `json:"amount"`
	Description    string     `json:"description"`
	RecordDate     time.Time  `json:"record_date"`
	RecordMonth    int        `json:"record_month"`
	RecordYear     int        `json:"record_year"`
	ApprovalStatus string     `json:"approval_status"`
	SubmittedBy    *string    `json:"submitted_by,omitempty"`
	ReviewedBy     *string    `json:"reviewed_by,omitempty"`
	ReviewedAt     *time.Time `json:"reviewed_at,omitempty"`
	ApprovedBy     *string    `json:"approved_by,omitempty"`
	ApprovedAt     *time.Time `json:"approved_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type FinanceSummaryMonth struct {
	Month   int   `json:"month"`
	Income  int64 `json:"income"`
	Outcome int64 `json:"outcome"`
}

type FinanceSummary struct {
	Year               int                   `json:"year"`
	Monthly            []FinanceSummaryMonth `json:"monthly"`
	TotalIncome        int64                 `json:"total_income"`
	TotalOutcome       int64                 `json:"total_outcome"`
	NetProfitThisMonth int64                 `json:"net_profit_this_month"`
	ByCategory         map[string]int64      `json:"by_category"`
}
