package marketing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	marketingdto "github.com/kana-consultant/kantor/backend/internal/dto/marketing"
	marketingservice "github.com/kana-consultant/kantor/backend/internal/service/marketing"
)

type errorResponse struct {
	Success bool `json:"success"`
	Error   struct {
		Code    string            `json:"code"`
		Message string            `json:"message"`
		Details map[string]string `json:"details"`
	} `json:"error"`
}

func decodeError(t *testing.T, recorder *httptest.ResponseRecorder) errorResponse {
	t.Helper()
	var body errorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not the error envelope: %v (%s)", err, recorder.Body.String())
	}
	if body.Success {
		t.Fatalf("error response has success=true: %s", recorder.Body.String())
	}
	return body
}

// Every error a caller can cause is a 4xx with its own code; details name
// the request field where there is one. Only an unmapped error is a 500, and
// its text is not sent to the client.
func TestCampaignErrorResponses(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		status  int
		code    string
		details map[string]string
	}{
		{"validation", &marketingservice.CampaignValidationError{Field: "end_date", Rule: "before_start_date", Message: "end_date must not be before start_date"},
			http.StatusBadRequest, "VALIDATION_ERROR", map[string]string{"end_date": "before_start_date"}},
		{"stage unavailable", &marketingservice.CampaignStageUnavailableError{Stage: "live", Cause: errors.New("insert refused")},
			http.StatusConflict, "CAMPAIGN_STAGE_UNAVAILABLE", map[string]string{"status": "stage_unavailable", "stage": "live"}},
		{"campaign not found", marketingservice.ErrCampaignNotFound, http.StatusNotFound, "CAMPAIGN_NOT_FOUND", nil},
		{"column not found", marketingservice.ErrCampaignColumnNotFound, http.StatusNotFound, "CAMPAIGN_COLUMN_NOT_FOUND", nil},
		{"attachment not found", marketingservice.ErrCampaignAttachmentNotFound, http.StatusNotFound, "CAMPAIGN_ATTACHMENT_NOT_FOUND", nil},
		{"pic not found", marketingservice.ErrCampaignPICNotFound, http.StatusBadRequest, "VALIDATION_ERROR", map[string]string{"pic_employee_id": "not found"}},
		{"column in use", marketingservice.ErrCampaignColumnInUse, http.StatusConflict, "CAMPAIGN_COLUMN_IN_USE", nil},
		{"stage column delete", marketingservice.ErrCampaignColumnProtected, http.StatusConflict, "CAMPAIGN_COLUMN_PROTECTED", nil},
		{"column name taken", marketingservice.ErrCampaignColumnNameTaken, http.StatusConflict, "CAMPAIGN_COLUMN_NAME_TAKEN", map[string]string{"name": "taken"}},
		{"wrapped sentinel", fmt.Errorf("move: %w", marketingservice.ErrCampaignColumnNotFound), http.StatusNotFound, "CAMPAIGN_COLUMN_NOT_FOUND", nil},
		{"unmapped", errors.New("pq: connection reset by peer"), http.StatusInternalServerError, "INTERNAL_ERROR", nil},
	}

	handler := &CampaignsHandler{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.writeError(context.Background(), recorder, tc.err)

			if recorder.Code != tc.status {
				t.Errorf("status = %d, want %d", recorder.Code, tc.status)
			}
			body := decodeError(t, recorder)
			if body.Error.Code != tc.code {
				t.Errorf("code = %q, want %q", body.Error.Code, tc.code)
			}
			if body.Error.Message == "" {
				t.Error("message is empty")
			}
			if len(tc.details) != 0 && !reflect.DeepEqual(body.Error.Details, tc.details) {
				t.Errorf("details = %v, want %v", body.Error.Details, tc.details)
			}
			if tc.status == http.StatusInternalServerError && strings.Contains(recorder.Body.String(), "connection reset") {
				t.Errorf("internal error text leaked to the client: %s", recorder.Body.String())
			}
		})
	}

	// The stage error names the stage in its message.
	recorder := httptest.NewRecorder()
	handler.writeError(context.Background(), recorder, &marketingservice.CampaignStageUnavailableError{Stage: "in_production"})
	if body := decodeError(t, recorder); !strings.Contains(body.Error.Message, `"in_production"`) {
		t.Errorf("stage error message does not name the stage: %q", body.Error.Message)
	}
}

func validate(t *testing.T, body string, target interface{}) (*httptest.ResponseRecorder, bool) {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	ok := decodeAndValidate(newValidator(), recorder, request, target)
	return recorder, ok
}

// Validation details of the marketing handlers use the JSON field names the
// client sent, and the rule that failed.
func TestValidationDetailsUseJSONFieldNames(t *testing.T) {
	valid := `{"name":"PRODUCT LAUNCH | TRAFFIC","channel":"meta_ads","budget_amount":700000,"pic_employee_id":null,
		"start_date":"2026-10-06","end_date":"2026-10-12","brief_text":null,"status":"live"}`
	var accepted marketingdto.CreateCampaignRequest
	if recorder, ok := validate(t, valid, &accepted); !ok {
		t.Fatalf("a valid Meta Ads campaign was rejected: %s", recorder.Body.String())
	}

	var rejected marketingdto.CreateCampaignRequest
	recorder, ok := validate(t, `{"name":"ab","channel":"billboard","budget_amount":-1,"pic_employee_id":"",
		"start_date":"2026-10-06T00:00:00Z","end_date":"","status":"running","description":"`+strings.Repeat("x", 5001)+`"}`, &rejected)
	if ok {
		t.Fatal("an invalid campaign was accepted")
	}
	if recorder.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", recorder.Code)
	}
	body := decodeError(t, recorder)
	want := map[string]string{
		"name":            "min",
		"channel":         "oneof",
		"budget_amount":   "min",
		"pic_employee_id": "uuid4",
		"start_date":      "datetime",
		"end_date":        "required",
		"status":          "oneof",
		"description":     "max",
	}
	if body.Error.Code != "VALIDATION_ERROR" || !reflect.DeepEqual(body.Error.Details, want) {
		t.Errorf("code %q details %v, want VALIDATION_ERROR %v", body.Error.Code, body.Error.Details, want)
	}

	var metric marketingdto.CreateAdsMetricRequest
	recorder, ok = validate(t, `{"campaign_id":"not-a-uuid","platform":"email","period_start":"2026-10-06","period_end":"2026-10-12"}`, &metric)
	if ok {
		t.Fatal("an invalid ads metric was accepted")
	}
	wantMetric := map[string]string{"campaign_id": "uuid4", "platform": "oneof"}
	if body := decodeError(t, recorder); !reflect.DeepEqual(body.Error.Details, wantMetric) {
		t.Errorf("ads metric details = %v, want %v", body.Error.Details, wantMetric)
	}
	if recorder, ok := validate(t, `{"campaign_id":"7f1f3c1e-6f0a-4d0b-9a55-0d7a4c2f9b11","platform":"meta_ads","period_start":"2026-10-06","period_end":"2026-10-12"}`, &metric); !ok {
		t.Errorf("an ads metric on meta_ads was rejected: %s", recorder.Body.String())
	}

	// Query filters report their query-string names.
	err := newValidator().Struct(marketingdto.ListCampaignsQuery{PerPage: 500, Channel: "billboard", DateFrom: "yesterday"})
	wantQuery := map[string]string{"per_page": "max", "channel": "oneof", "date_from": "datetime"}
	if got := validationDetails(err); !reflect.DeepEqual(got, wantQuery) {
		t.Errorf("query details = %v, want %v", got, wantQuery)
	}

	if recorder, ok := validate(t, `{"name":`, &accepted); ok || decodeError(t, recorder).Error.Code != "INVALID_JSON" {
		t.Errorf("broken JSON must be INVALID_JSON: %s", recorder.Body.String())
	}
}

// The PIC picker is a campaign route: it must exist, sit before the
// /{campaignID} pattern, and not be reachable as a campaign id.
func TestCampaignRoutesIncludePICOptions(t *testing.T) {
	router := chi.NewRouter()
	NewCampaignsHandler(nil, t.TempDir(), nil).RegisterRoutes(router)

	routes := map[string]bool{}
	if err := chi.Walk(router, func(method string, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		routes[method+" "+route] = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"GET /pic-options", "GET /kanban", "GET /{campaignID}", "POST /", "PUT /{campaignID}", "PATCH /{campaignID}/move"} {
		if !routes[want] {
			t.Errorf("route %s is not registered (have %v)", want, routes)
		}
	}

	routeContext := chi.NewRouteContext()
	if !router.Match(routeContext, http.MethodGet, "/pic-options") || routeContext.RoutePattern() != "/pic-options" {
		t.Errorf("GET /pic-options resolves to %q, want the static route", routeContext.RoutePattern())
	}
}
