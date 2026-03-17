package marketing

import "time"

type CreateAdsMetricRequest struct {
	CampaignID  string    `json:"campaign_id" validate:"required,uuid4"`
	Platform    string    `json:"platform" validate:"required,oneof=instagram facebook google_ads tiktok youtube other"`
	PeriodStart time.Time `json:"period_start" validate:"required"`
	PeriodEnd   time.Time `json:"period_end" validate:"required"`
	AmountSpent int64     `json:"amount_spent" validate:"min=0"`
	Impressions int64     `json:"impressions" validate:"min=0"`
	Clicks      int64     `json:"clicks" validate:"min=0"`
	Conversions int64     `json:"conversions" validate:"min=0"`
	Revenue     int64     `json:"revenue" validate:"min=0"`
	Notes       *string   `json:"notes"`
}

type UpdateAdsMetricRequest = CreateAdsMetricRequest

type BatchCreateAdsMetricsRequest struct {
	Entries []CreateAdsMetricRequest `json:"entries" validate:"required,min=1,max=100,dive"`
}

type ListAdsMetricsQuery struct {
	Page      int    `validate:"omitempty,min=1"`
	PerPage   int    `validate:"omitempty,min=1,max=100"`
	CampaignID string `validate:"omitempty,uuid4"`
	Platform  string `validate:"omitempty,oneof=instagram facebook google_ads tiktok youtube other"`
	DateFrom  string `validate:"omitempty,datetime=2006-01-02"`
	DateTo    string `validate:"omitempty,datetime=2006-01-02"`
}

type AdsMetricsSummaryQuery struct {
	GroupBy  string `validate:"required,oneof=campaign platform month"`
	DateFrom string `validate:"omitempty,datetime=2006-01-02"`
	DateTo   string `validate:"omitempty,datetime=2006-01-02"`
}
