package model

import "time"

type AdsMetric struct {
	ID          string     `json:"id"`
	CampaignID  string     `json:"campaign_id"`
	CampaignName *string   `json:"campaign_name,omitempty"`
	Platform    string     `json:"platform"`
	PeriodStart time.Time  `json:"period_start"`
	PeriodEnd   time.Time  `json:"period_end"`
	AmountSpent int64      `json:"amount_spent"`
	Impressions int64      `json:"impressions"`
	Clicks      int64      `json:"clicks"`
	Conversions int64      `json:"conversions"`
	Revenue     int64      `json:"revenue"`
	Notes       *string    `json:"notes,omitempty"`
	CreatedBy   string     `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CPR         *float64   `json:"cpr,omitempty"`
	ROAS        *float64   `json:"roas,omitempty"`
	CTR         *float64   `json:"ctr,omitempty"`
	CPC         *float64   `json:"cpc,omitempty"`
	CPM         *float64   `json:"cpm,omitempty"`
}

type AdsMetricsSummaryRow struct {
	GroupKey         string   `json:"group_key"`
	GroupLabel       string   `json:"group_label"`
	TotalSpent       int64    `json:"total_spent"`
	TotalImpressions int64    `json:"total_impressions"`
	TotalClicks      int64    `json:"total_clicks"`
	TotalConversions int64    `json:"total_conversions"`
	TotalRevenue     int64    `json:"total_revenue"`
	CPR              *float64 `json:"cpr,omitempty"`
	ROAS             *float64 `json:"roas,omitempty"`
	CTR              *float64 `json:"ctr,omitempty"`
	CPC              *float64 `json:"cpc,omitempty"`
	CPM              *float64 `json:"cpm,omitempty"`
}

type AdsMetricsSummary struct {
	GroupBy string                 `json:"group_by"`
	Items   []AdsMetricsSummaryRow `json:"items"`
}
