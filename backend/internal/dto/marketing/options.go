package marketing

import (
	"strings"

	"github.com/go-playground/validator/v10"
)

// The value lists below are the single backend definition of the campaign
// channels, the ads-metrics platforms and the campaign stages. The request
// validators (through the tags registered by RegisterValidations) and the
// MCP tool hints read them, so adding a value here is the only code change
// needed. The CHECK constraints chk_campaigns_channel,
// chk_ads_metrics_platform, chk_campaigns_status and
// chk_campaign_columns_stage must list the same values.
var (
	// CampaignChannels are the values accepted for campaigns.channel.
	CampaignChannels = []string{"meta_ads", "instagram", "facebook", "google_ads", "tiktok", "youtube", "email", "other"}

	// AdsMetricPlatforms are the values accepted for ads_metrics.platform:
	// the campaign channels that are ad platforms (no "email").
	AdsMetricPlatforms = []string{"meta_ads", "instagram", "facebook", "google_ads", "tiktok", "youtube", "other"}

	// CampaignStages are the values of campaigns.status, in board order.
	// Exactly one board column per tenant carries each of them as its stage.
	CampaignStages = []string{"ideation", "planning", "in_production", "live", "completed", "archived"}
)

// Validator tags backed by the lists above.
const (
	CampaignChannelTag   = "campaign_channel"
	AdsMetricPlatformTag = "ads_platform"
	CampaignStageTag     = "campaign_stage"
)

// RegisterValidations registers the three tags on v as aliases of `oneof`
// over the shared lists. Every validator that validates a marketing DTO
// must call it once (the marketing handlers do, in newValidator).
func RegisterValidations(v *validator.Validate) {
	v.RegisterAlias(CampaignChannelTag, oneOf(CampaignChannels))
	v.RegisterAlias(AdsMetricPlatformTag, oneOf(AdsMetricPlatforms))
	v.RegisterAlias(CampaignStageTag, oneOf(CampaignStages))
}

func oneOf(values []string) string {
	return "oneof=" + strings.Join(values, " ")
}
