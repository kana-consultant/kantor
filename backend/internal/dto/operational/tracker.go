package operational

import "time"

type TrackerHeartbeatRequest struct {
	SessionID string    `json:"session_id" validate:"required,uuid4"`
	URL       string    `json:"url" validate:"required,url"`
	Domain    string    `json:"domain" validate:"required,max=255"`
	PageTitle *string   `json:"page_title,omitempty" validate:"omitempty,max=500"`
	IsIdle    bool      `json:"is_idle"`
	Timestamp time.Time `json:"timestamp" validate:"required"`
}

type TrackerBatchEntriesRequest struct {
	Entries []TrackerHeartbeatRequest `json:"entries" validate:"required,min=1,dive"`
}

type TrackerConsentRequest struct{}

type TrackerEndSessionRequest struct {
	Timestamp *time.Time `json:"timestamp,omitempty"`
}

type DomainCategoryRequest struct {
	DomainPattern string `json:"domain_pattern" validate:"required,max=255"`
	Category      string `json:"category" validate:"required,oneof=work communication social_media entertainment development design documentation other uncategorized"`
	IsProductive  bool   `json:"is_productive"`
}
