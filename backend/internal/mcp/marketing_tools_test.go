package mcp_test

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/kana-consultant/kantor/backend/internal/app"
	"github.com/kana-consultant/kantor/backend/internal/config"
	marketingdto "github.com/kana-consultant/kantor/backend/internal/dto/marketing"
	"github.com/kana-consultant/kantor/backend/internal/mcp"
)

// The marketing tools tell an AI client which channels, platforms and stages
// exist (Meta Ads included) and how the board columns behave. The values come
// from the lists the request validators use, and every hint must belong to a
// route that is really mounted.
func TestMarketingToolsExposeAllowedValues(t *testing.T) {
	routes, err := app.BuildRouteTableForInspection(config.Config{AppEnv: "test"})
	if err != nil {
		t.Fatalf("build real router: %v", err)
	}
	tools, err := mcp.BuildCatalog(routes)
	if err != nil {
		t.Fatalf("BuildCatalog: %v", err)
	}
	byName := make(map[string]mcp.ToolSpec, len(tools))
	for _, tool := range tools {
		byName[tool.Name] = tool
	}

	enumOf := func(toolName string, param string) []string {
		t.Helper()
		tool, ok := byName[toolName]
		if !ok || tool.Meta == nil {
			t.Fatalf("tool %s is missing or has no hints", toolName)
		}
		for _, query := range tool.Meta.Query {
			if query.Name == param {
				return query.Enum
			}
		}
		t.Fatalf("tool %s has no query parameter %q", toolName, param)
		return nil
	}
	if got := enumOf("get_marketing_campaigns", "channel"); !reflect.DeepEqual(got, marketingdto.CampaignChannels) {
		t.Errorf("campaign channel enum = %v, want %v", got, marketingdto.CampaignChannels)
	}
	if got := enumOf("get_marketing_campaigns", "status"); !reflect.DeepEqual(got, marketingdto.CampaignStages) {
		t.Errorf("campaign stage enum = %v, want %v", got, marketingdto.CampaignStages)
	}
	if got := enumOf("get_marketing_ads_metrics", "platform"); !reflect.DeepEqual(got, marketingdto.AdsMetricPlatforms) {
		t.Errorf("ads platform enum = %v, want %v", got, marketingdto.AdsMetricPlatforms)
	}
	if got := enumOf("get_marketing_ads_metrics_summary", "group_by"); !reflect.DeepEqual(got, []string{"campaign", "platform", "month"}) {
		t.Errorf("group_by enum = %v", got)
	}

	hints := map[string][]string{
		"post_marketing_campaigns":                  {`"meta_ads"`, `"live"`, "pic_employee_id"},
		"put_marketing_campaigns_campaignid":        {`"meta_ads"`, "FULL replacement"},
		"post_marketing_ads_metrics":                {`"meta_ads"`, "campaign_id"},
		"put_marketing_ads_metrics_metricid":        {`"meta_ads"`},
		"get_marketing_campaigns_pic_options":       {"person in charge"},
		"patch_marketing_campaigns_campaignid_move": {"custom column", "column_id"},
		"get_marketing_columns":                     {"stage"},
		"post_marketing_columns":                    {"custom column"},
		"put_marketing_columns_columnid":            {"keeps its stage"},
		"delete_marketing_columns_columnid":         {"cannot be deleted", "CAMPAIGN_COLUMN_PROTECTED"},
		"patch_marketing_columns_reorder":           {"EVERY column"},
	}
	for name, fragments := range hints {
		tool, ok := byName[name]
		if !ok {
			t.Errorf("expected tool %s in the catalog", name)
			continue
		}
		if tool.Meta == nil {
			t.Errorf("tool %s has no hints: its annotation key does not match the mounted route", name)
			continue
		}
		text := tool.Meta.Description + " " + tool.Meta.Body
		for _, fragment := range fragments {
			if !strings.Contains(text, fragment) {
				t.Errorf("tool %s does not mention %q: %s", name, fragment, text)
			}
		}
	}

	// The PIC picker returns names and positions only; it is an ordinary
	// read tool, mounted under the campaigns routes.
	mounted := map[string]bool{}
	if err := chi.Walk(routes, func(method string, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		mounted[method+" "+strings.TrimSuffix(route, "/")] = true
		return nil
	}); err != nil {
		t.Fatalf("walk router: %v", err)
	}
	if !mounted["GET /api/v1/marketing/campaigns/pic-options"] {
		t.Error("GET /api/v1/marketing/campaigns/pic-options is not mounted")
	}
}
