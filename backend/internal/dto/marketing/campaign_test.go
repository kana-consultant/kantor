package marketing

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
)

func testValidator() *validator.Validate {
	v := validator.New(validator.WithRequiredStructEnabled())
	RegisterValidations(v)
	return v
}

// reportPayload has the shape of the request in the production report (the
// campaign name is replaced by a neutral one): a campaign for Meta ads
// entered with the channel "Other" because Meta Ads did not exist yet, stage
// Live, no PIC, an empty brief and a three-line description with a URL.
const reportPayload = `{
	"name": "PRODUCT LAUNCH | TRAFFIC",
	"description": "Traffic campaign for the product launch.\nLanding page: https://example.test/product-launch?utm_source=meta\nOptimise for link clicks.",
	"channel": "other",
	"budget_amount": 700000,
	"budget_currency": "IDR",
	"pic_employee_id": null,
	"start_date": "2026-10-06",
	"end_date": "2026-10-12",
	"brief_text": null,
	"status": "live"
}`

func decodeCampaign(t *testing.T, body string) CreateCampaignRequest {
	t.Helper()
	var request CreateCampaignRequest
	if err := json.Unmarshal([]byte(body), &request); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return request
}

func TestReportPayloadIsValid(t *testing.T) {
	v := testValidator()
	request := decodeCampaign(t, reportPayload)
	if err := v.Struct(request); err != nil {
		t.Fatalf("the payload of the report must pass validation: %v", err)
	}

	// The same campaign with the channel the marketing lead asked for.
	request.Channel = "meta_ads"
	if err := v.Struct(request); err != nil {
		t.Fatalf("channel meta_ads must pass validation: %v", err)
	}

	update := UpdateCampaignRequest(request)
	if err := v.Struct(update); err != nil {
		t.Fatalf("the same body must be valid for an update: %v", err)
	}
}

func TestCampaignRequestRejectsInvalidValues(t *testing.T) {
	v := testValidator()
	long := func(n int) *string {
		value := strings.Repeat("x", n)
		return &value
	}
	cases := map[string]struct {
		change func(*CreateCampaignRequest)
		field  string
	}{
		"unknown channel":      {func(r *CreateCampaignRequest) { r.Channel = "billboard" }, "Channel"},
		"unknown stage":        {func(r *CreateCampaignRequest) { r.Status = "running" }, "Status"},
		"empty stage":          {func(r *CreateCampaignRequest) { r.Status = "" }, "Status"},
		"negative budget":      {func(r *CreateCampaignRequest) { r.BudgetAmount = -1 }, "BudgetAmount"},
		"datetime as date":     {func(r *CreateCampaignRequest) { r.EndDate = "2026-10-12T00:00:00Z" }, "EndDate"},
		"description too long": {func(r *CreateCampaignRequest) { r.Description = long(5001) }, "Description"},
		"brief too long":       {func(r *CreateCampaignRequest) { r.BriefText = long(20001) }, "BriefText"},
		"name too short":       {func(r *CreateCampaignRequest) { r.Name = "ab" }, "Name"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			request := decodeCampaign(t, reportPayload)
			tc.change(&request)
			err := v.Struct(request)
			validationErrors, ok := err.(validator.ValidationErrors)
			if !ok || len(validationErrors) != 1 || validationErrors[0].StructField() != tc.field {
				t.Fatalf("want exactly one error on %s, got %v", tc.field, err)
			}
		})
	}

	// The limits themselves are accepted.
	request := decodeCampaign(t, reportPayload)
	request.Description = long(5000)
	request.BriefText = long(20000)
	if err := v.Struct(request); err != nil {
		t.Errorf("values at the length limits must pass: %v", err)
	}
}

func TestSharedListsDriveEveryValidator(t *testing.T) {
	v := testValidator()

	for _, channel := range CampaignChannels {
		request := decodeCampaign(t, reportPayload)
		request.Channel = channel
		if err := v.Struct(request); err != nil {
			t.Errorf("campaign channel %q rejected on create: %v", channel, err)
		}
		if err := v.Struct(UpdateCampaignRequest(request)); err != nil {
			t.Errorf("campaign channel %q rejected on update: %v", channel, err)
		}
		if err := v.Struct(ListCampaignsQuery{Channel: channel}); err != nil {
			t.Errorf("campaign channel %q rejected as list filter: %v", channel, err)
		}
	}
	for _, stage := range CampaignStages {
		request := decodeCampaign(t, reportPayload)
		request.Status = stage
		if err := v.Struct(request); err != nil {
			t.Errorf("stage %q rejected on create: %v", stage, err)
		}
		if err := v.Struct(ListCampaignsQuery{Status: stage}); err != nil {
			t.Errorf("stage %q rejected as list filter: %v", stage, err)
		}
	}
	for _, platform := range AdsMetricPlatforms {
		metric := CreateAdsMetricRequest{
			CampaignID:  "7f1f3c1e-6f0a-4d0b-9a55-0d7a4c2f9b11",
			Platform:    platform,
			PeriodStart: "2026-10-06",
			PeriodEnd:   "2026-10-12",
		}
		if err := v.Struct(metric); err != nil {
			t.Errorf("platform %q rejected on create: %v", platform, err)
		}
		if err := v.Struct(BatchCreateAdsMetricsRequest{Entries: []CreateAdsMetricRequest{metric}}); err != nil {
			t.Errorf("platform %q rejected in a batch: %v", platform, err)
		}
		if err := v.Struct(ListAdsMetricsQuery{Platform: platform}); err != nil {
			t.Errorf("platform %q rejected as list filter: %v", platform, err)
		}
	}

	// The filters stay optional and still refuse unknown values.
	if err := v.Struct(ListCampaignsQuery{}); err != nil {
		t.Errorf("empty campaign filters must pass: %v", err)
	}
	if err := v.Struct(ListCampaignsQuery{Channel: "billboard"}); err == nil {
		t.Error("unknown channel filter must be rejected")
	}
	if err := v.Struct(ListAdsMetricsQuery{Platform: "email"}); err == nil {
		t.Error("email is a campaign channel, not an ads platform")
	}
}

func TestSharedListsContents(t *testing.T) {
	contains := func(values []string, want string) bool {
		for _, value := range values {
			if value == want {
				return true
			}
		}
		return false
	}

	// Meta Ads is added; nothing that existing rows use is removed.
	for _, channel := range []string{"meta_ads", "instagram", "facebook", "google_ads", "tiktok", "youtube", "email", "other"} {
		if !contains(CampaignChannels, channel) {
			t.Errorf("campaign channel %q is missing", channel)
		}
	}
	// Ads platforms are the campaign channels without email.
	var withoutEmail []string
	for _, channel := range CampaignChannels {
		if channel != "email" {
			withoutEmail = append(withoutEmail, channel)
		}
	}
	if !reflect.DeepEqual(AdsMetricPlatforms, withoutEmail) {
		t.Errorf("AdsMetricPlatforms = %v, want the campaign channels without email %v", AdsMetricPlatforms, withoutEmail)
	}
	want := []string{"ideation", "planning", "in_production", "live", "completed", "archived"}
	if !reflect.DeepEqual(CampaignStages, want) {
		t.Errorf("CampaignStages = %v, want %v", CampaignStages, want)
	}
}

// No marketing DTO may fall back to a literal channel/platform/stage list:
// a literal list is how the lists drifted apart before.
func TestNoLiteralChannelOrStageListsInTags(t *testing.T) {
	types := []reflect.Type{
		reflect.TypeOf(CreateCampaignRequest{}),
		reflect.TypeOf(UpdateCampaignRequest{}),
		reflect.TypeOf(ListCampaignsQuery{}),
		reflect.TypeOf(CreateAdsMetricRequest{}),
		reflect.TypeOf(ListAdsMetricsQuery{}),
	}
	want := map[string]string{"Channel": CampaignChannelTag, "Platform": AdsMetricPlatformTag, "Status": CampaignStageTag}
	for _, typ := range types {
		for index := 0; index < typ.NumField(); index++ {
			field := typ.Field(index)
			tag, tracked := want[field.Name]
			if !tracked {
				continue
			}
			rule := field.Tag.Get("validate")
			if !strings.Contains(rule, tag) || strings.Contains(rule, "oneof=") {
				t.Errorf("%s.%s validate tag is %q, want the shared tag %q", typ.Name(), field.Name, rule, tag)
			}
		}
	}
}
