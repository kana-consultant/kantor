package model

import "time"

type OverviewSeriesPoint struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Value int64  `json:"value"`
}

type OperationalRecentTask struct {
	ID             string     `json:"id"`
	ProjectID      string     `json:"project_id"`
	ProjectName    string     `json:"project_name"`
	Title          string     `json:"title"`
	Status         string     `json:"status"`
	Priority       string     `json:"priority"`
	AssigneeID     *string    `json:"assignee_id,omitempty"`
	AssigneeName   *string    `json:"assignee_name,omitempty"`
	AssigneeAvatar *string    `json:"assignee_avatar,omitempty"`
	DueDate        *time.Time `json:"due_date,omitempty"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type OperationalOverview struct {
	TotalProjects   int64                   `json:"total_projects"`
	ActiveTasks     int64                   `json:"active_tasks"`
	OverdueTasks    int64                   `json:"overdue_tasks"`
	TeamMembers     int64                   `json:"team_members"`
	CompletedByWeek []OverviewSeriesPoint   `json:"completed_by_week"`
	RecentTasks     []OperationalRecentTask `json:"recent_tasks"`
}

type FinanceOverviewPoint struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Income  int64  `json:"income"`
	Outcome int64  `json:"outcome"`
}

type HrisUpcomingRenewal struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Vendor          string    `json:"vendor"`
	RenewalDate     time.Time `json:"renewal_date"`
	DaysRemaining   int       `json:"days_remaining"`
	CostAmount      int64     `json:"cost_amount"`
	CostCurrency    string    `json:"cost_currency"`
	PICEmployeeName *string   `json:"pic_employee_name,omitempty"`
}

type HrisOverviewSalaryHistoryRow struct {
	EmployeeID         string
	EffectiveDate      time.Time
	NetSalaryEncrypted string
}

type HrisOverviewSubscriptionRow struct {
	StartDate    time.Time
	BillingCycle string
	CostAmount   int64
}

type HrisOverview struct {
	TotalEmployees                int64                  `json:"total_employees"`
	TotalMonthlyPayroll           int64                  `json:"total_monthly_payroll"`
	ActiveSubscriptions           int64                  `json:"active_subscriptions"`
	ActiveSubscriptionMonthlyCost int64                  `json:"active_subscription_monthly_cost"`
	MonthlyNet                    int64                  `json:"monthly_net"`
	PendingReimbursements         int64                  `json:"pending_reimbursements"`
	MonthlyReimbursementTotal     int64                  `json:"monthly_reimbursement_total"`
	IncomeVsOutcome               []FinanceOverviewPoint `json:"income_vs_outcome"`
	UpcomingRenewals              []HrisUpcomingRenewal  `json:"upcoming_renewals"`
	RecentReimbursements          []Reimbursement        `json:"recent_reimbursements"`
}

type MarketingROASTrendPoint struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Spent   int64    `json:"spent"`
	Revenue int64    `json:"revenue"`
	ROAS    *float64 `json:"roas,omitempty"`
}

type MarketingTopCampaign struct {
	CampaignID   string   `json:"campaign_id"`
	CampaignName string   `json:"campaign_name"`
	Status       string   `json:"status"`
	TotalSpent   int64    `json:"total_spent"`
	TotalRevenue int64    `json:"total_revenue"`
	ROAS         *float64 `json:"roas,omitempty"`
}

type MarketingOverview struct {
	ActiveCampaigns int64                     `json:"active_campaigns"`
	TotalAdsSpent   int64                     `json:"total_ads_spent"`
	OverallROAS     *float64                  `json:"overall_roas,omitempty"`
	TotalLeads      int64                     `json:"total_leads"`
	ConversionRate  float64                   `json:"conversion_rate"`
	ROASTrend       []MarketingROASTrendPoint `json:"roas_trend"`
	LeadsByStage    []LeadSummaryRow          `json:"leads_by_stage"`
	TopCampaigns    []MarketingTopCampaign    `json:"top_campaigns"`
}
