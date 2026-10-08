package marketing

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	marketingdto "github.com/kana-consultant/kantor/backend/internal/dto/marketing"
	"github.com/kana-consultant/kantor/backend/internal/model"
	marketingrepo "github.com/kana-consultant/kantor/backend/internal/repository/marketing"
	notificationsrepo "github.com/kana-consultant/kantor/backend/internal/repository/notifications"
)

// fakeCampaignsRepo records the writes of the service and returns canned
// results. Methods the tests do not reach panic through the nil embedded
// interface, which is the wanted failure for an unexpected call.
type fakeCampaignsRepo struct {
	campaignsRepository

	campaign       model.Campaign // returned by GetCampaignByID before a move
	moved          model.Campaign // returned by MoveCampaign and by GetCampaignByID after it
	previousStatus string

	createErr      error
	updateErr      error
	logActivityErr error

	created      []marketingrepo.UpsertCampaignParams
	updated      []marketingrepo.UpsertCampaignParams
	moves        int
	activities   []string
	columnWrites []string
}

func (f *fakeCampaignsRepo) CreateCampaign(_ context.Context, params marketingrepo.UpsertCampaignParams) (model.Campaign, error) {
	if f.createErr != nil {
		return model.Campaign{}, f.createErr
	}
	f.created = append(f.created, params)
	return model.Campaign{ID: "campaign-1", Name: params.Name, Status: params.Status}, nil
}

func (f *fakeCampaignsRepo) UpdateCampaign(_ context.Context, campaignID string, params marketingrepo.UpsertCampaignParams) (model.Campaign, string, error) {
	if f.updateErr != nil {
		return model.Campaign{}, "", f.updateErr
	}
	f.updated = append(f.updated, params)
	f.campaign = model.Campaign{ID: campaignID, Name: params.Name, Status: params.Status}
	return f.campaign, f.previousStatus, nil
}

func (f *fakeCampaignsRepo) GetCampaignByID(_ context.Context, campaignID string) (model.Campaign, error) {
	if f.moves > 0 {
		return f.moved, nil
	}
	if f.campaign.ID == "" {
		return model.Campaign{ID: campaignID, Status: "live"}, nil
	}
	return f.campaign, nil
}

func (f *fakeCampaignsRepo) ListAttachments(context.Context, string) ([]model.CampaignAttachment, error) {
	return []model.CampaignAttachment{}, nil
}

func (f *fakeCampaignsRepo) MoveCampaign(context.Context, string, string, int, string) (model.Campaign, error) {
	f.moves++
	return f.moved, nil
}

func (f *fakeCampaignsRepo) LogActivity(_ context.Context, _ string, _ string, action string, _ map[string]any) error {
	if f.logActivityErr != nil {
		return f.logActivityErr
	}
	f.activities = append(f.activities, action)
	return nil
}

func (f *fakeCampaignsRepo) CreateColumn(_ context.Context, params marketingrepo.CreateCampaignColumnParams) (model.CampaignColumn, error) {
	f.columnWrites = append(f.columnWrites, params.Name)
	return model.CampaignColumn{Name: params.Name}, nil
}

func (f *fakeCampaignsRepo) UpdateColumn(_ context.Context, _ string, params marketingrepo.UpdateCampaignColumnParams) (model.CampaignColumn, error) {
	f.columnWrites = append(f.columnWrites, params.Name)
	return model.CampaignColumn{Name: params.Name}, nil
}

func (f *fakeCampaignsRepo) BeginTx(context.Context) (pgx.Tx, error) {
	return nil, errors.New("not used")
}

type fakeAuthRepo struct{ err error }

func (f fakeAuthRepo) ListUserIDsByPermission(context.Context, string) ([]string, error) {
	return []string{"editor-1", "editor-2"}, f.err
}

type fakeNotifications struct {
	err  error
	sent [][]notificationsrepo.CreateParams
}

func (f *fakeNotifications) CreateMany(_ context.Context, params []notificationsrepo.CreateParams) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, params)
	return nil
}

func reportRequest() marketingdto.CreateCampaignRequest {
	return marketingdto.CreateCampaignRequest{
		Name:         "PRODUCT LAUNCH | TRAFFIC",
		Channel:      "other",
		BudgetAmount: 700000,
		StartDate:    "2026-10-06",
		EndDate:      "2026-10-12",
		Status:       "live",
	}
}

func wantValidationError(t *testing.T, err error, field string, rule string) {
	t.Helper()
	var validationErr *CampaignValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("want a CampaignValidationError on %s, got %v", field, err)
	}
	if validationErr.Field != field || validationErr.Rule != rule || validationErr.Message == "" {
		t.Errorf("validation error = %+v, want field %q rule %q and a message", validationErr, field, rule)
	}
}

func TestCreateCampaignValidatesAfterTrimming(t *testing.T) {
	cases := map[string]struct {
		change func(*marketingdto.CreateCampaignRequest)
		field  string
		rule   string
	}{
		"name of spaces":        {func(r *marketingdto.CreateCampaignRequest) { r.Name = "     " }, "name", "min"},
		"name too short padded": {func(r *marketingdto.CreateCampaignRequest) { r.Name = "  ab  " }, "name", "min"},
		"end before start":      {func(r *marketingdto.CreateCampaignRequest) { r.EndDate = "2026-10-05" }, "end_date", "before_start_date"},
		"unparseable start":     {func(r *marketingdto.CreateCampaignRequest) { r.StartDate = "soon" }, "start_date", "datetime"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &fakeCampaignsRepo{}
			service := NewCampaignsService(repo, nil, nil)
			request := reportRequest()
			tc.change(&request)

			_, err := service.CreateCampaign(context.Background(), request, "user-1")
			wantValidationError(t, err, tc.field, tc.rule)
			if len(repo.created) != 0 {
				t.Error("an invalid campaign reached the repository")
			}

			_, _, err = service.UpdateCampaign(context.Background(), "campaign-1", marketingdto.UpdateCampaignRequest(request), "user-1")
			wantValidationError(t, err, tc.field, tc.rule)
			if len(repo.updated) != 0 {
				t.Error("an invalid update reached the repository")
			}
		})
	}
}

func TestCreateCampaignPassesTheReportPayload(t *testing.T) {
	repo := &fakeCampaignsRepo{}
	notifications := &fakeNotifications{}
	service := NewCampaignsService(repo, fakeAuthRepo{}, notifications)

	request := reportRequest()
	request.Name = "  PRODUCT LAUNCH | TRAFFIC  "
	request.EndDate = request.StartDate // a one-day campaign is valid
	detail, err := service.CreateCampaign(context.Background(), request, "user-1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(repo.created) != 1 {
		t.Fatalf("repository creates = %d", len(repo.created))
	}
	params := repo.created[0]
	if params.Name != "PRODUCT LAUNCH | TRAFFIC" || params.Status != "live" || params.BudgetCurrency != "IDR" || params.ActorID != "user-1" {
		t.Errorf("repository params = %+v", params)
	}
	if !params.StartDate.Equal(params.EndDate) {
		t.Errorf("dates = %v .. %v", params.StartDate, params.EndDate)
	}
	if detail.Attachments == nil {
		t.Error("attachments must be an empty list, not null")
	}
	// Notification behaviour is unchanged: creating a campaign, even as
	// live, sends none.
	if len(notifications.sent) != 0 {
		t.Errorf("create sent %d notifications, want none", len(notifications.sent))
	}
}

func TestUpdateCampaignReportsThePreviousStage(t *testing.T) {
	repo := &fakeCampaignsRepo{previousStatus: "planning"}
	notifications := &fakeNotifications{}
	service := NewCampaignsService(repo, fakeAuthRepo{}, notifications)

	detail, previous, err := service.UpdateCampaign(context.Background(), "campaign-1", marketingdto.UpdateCampaignRequest(reportRequest()), "user-1")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if previous != "planning" || detail.Campaign.Status != "live" {
		t.Errorf("previous %q, status %q", previous, detail.Campaign.Status)
	}
	// An edit writes no activity row of its own (the request audit row is
	// the single entry) and, as before, sends no notification.
	if len(repo.activities) != 0 || len(notifications.sent) != 0 {
		t.Errorf("update wrote %v activities and %d notifications, want none", repo.activities, len(notifications.sent))
	}
}

func TestMoveCampaignSideEffectsAreBestEffort(t *testing.T) {
	live := model.Campaign{ID: "campaign-1", Name: "Launch", Status: "live", CreatedBy: "creator-1"}
	planning := model.Campaign{ID: "campaign-1", Name: "Launch", Status: "planning", CreatedBy: "creator-1"}
	request := marketingdto.MoveCampaignRequest{ColumnID: "column-1", Position: 1}

	t.Run("move to live logs once and notifies", func(t *testing.T) {
		repo := &fakeCampaignsRepo{campaign: planning, moved: live}
		notifications := &fakeNotifications{}
		service := NewCampaignsService(repo, fakeAuthRepo{}, notifications)

		detail, err := service.MoveCampaign(context.Background(), "campaign-1", request, "user-1")
		if err != nil || detail.Campaign.Status != "live" {
			t.Fatalf("move: %v, status %q", err, detail.Campaign.Status)
		}
		if len(repo.activities) != 1 || repo.activities[0] != "campaign_moved" {
			t.Errorf("activities = %v, want exactly one campaign_moved", repo.activities)
		}
		if len(notifications.sent) != 1 || len(notifications.sent[0]) != 3 {
			t.Errorf("notifications = %+v, want one batch to the two editors and the creator", notifications.sent)
		}
	})

	t.Run("move within live does not notify again", func(t *testing.T) {
		repo := &fakeCampaignsRepo{campaign: live, moved: live}
		notifications := &fakeNotifications{}
		service := NewCampaignsService(repo, fakeAuthRepo{}, notifications)
		if _, err := service.MoveCampaign(context.Background(), "campaign-1", request, "user-1"); err != nil {
			t.Fatal(err)
		}
		if len(notifications.sent) != 0 {
			t.Errorf("notifications = %d, want none", len(notifications.sent))
		}
	})

	t.Run("a failed activity entry does not fail the move", func(t *testing.T) {
		repo := &fakeCampaignsRepo{campaign: planning, moved: live, logActivityErr: errors.New("audit table unavailable")}
		notifications := &fakeNotifications{}
		service := NewCampaignsService(repo, fakeAuthRepo{}, notifications)

		detail, err := service.MoveCampaign(context.Background(), "campaign-1", request, "user-1")
		if err != nil {
			t.Fatalf("the move is committed, so it must be reported as done: %v", err)
		}
		if detail.Campaign.Status != "live" || repo.moves != 1 {
			t.Errorf("status %q, moves %d", detail.Campaign.Status, repo.moves)
		}
		if len(notifications.sent) != 1 {
			t.Error("the notification must still be attempted after a failed activity entry")
		}
	})

	t.Run("a failed notification does not fail the move", func(t *testing.T) {
		for name, service := range map[string]*CampaignsService{
			"recipient lookup fails": NewCampaignsService(&fakeCampaignsRepo{campaign: planning, moved: live}, fakeAuthRepo{err: errors.New("rbac down")}, &fakeNotifications{}),
			"sending fails":          NewCampaignsService(&fakeCampaignsRepo{campaign: planning, moved: live}, fakeAuthRepo{}, &fakeNotifications{err: errors.New("notifications down")}),
		} {
			detail, err := service.MoveCampaign(context.Background(), "campaign-1", request, "user-1")
			if err != nil || detail.Campaign.Status != "live" {
				t.Errorf("%s: err %v, status %q", name, err, detail.Campaign.Status)
			}
		}
	})
}

func TestColumnNamesAreValidatedAfterTrimming(t *testing.T) {
	repo := &fakeCampaignsRepo{}
	service := NewCampaignsService(repo, nil, nil)

	_, err := service.CreateColumn(context.Background(), marketingdto.CreateCampaignColumnRequest{Name: "   "})
	wantValidationError(t, err, "name", "min")
	_, err = service.UpdateColumn(context.Background(), "column-1", marketingdto.UpdateCampaignColumnRequest{Name: " x "})
	wantValidationError(t, err, "name", "min")
	if len(repo.columnWrites) != 0 {
		t.Errorf("invalid column names reached the repository: %v", repo.columnWrites)
	}

	if _, err := service.CreateColumn(context.Background(), marketingdto.CreateCampaignColumnRequest{Name: "  Review Klien "}); err != nil {
		t.Fatal(err)
	}
	if len(repo.columnWrites) != 1 || repo.columnWrites[0] != "Review Klien" {
		t.Errorf("column writes = %v", repo.columnWrites)
	}
}

func TestMapCampaignError(t *testing.T) {
	if mapCampaignError(nil) != nil {
		t.Error("nil must stay nil")
	}

	sentinels := map[error]error{
		marketingrepo.ErrCampaignNotFound:           ErrCampaignNotFound,
		marketingrepo.ErrCampaignColumnNotFound:     ErrCampaignColumnNotFound,
		marketingrepo.ErrCampaignAttachmentNotFound: ErrCampaignAttachmentNotFound,
		marketingrepo.ErrCampaignPICNotFound:        ErrCampaignPICNotFound,
		marketingrepo.ErrCampaignColumnInUse:        ErrCampaignColumnInUse,
		marketingrepo.ErrCampaignColumnProtected:    ErrCampaignColumnProtected,
		marketingrepo.ErrCampaignColumnNameTaken:    ErrCampaignColumnNameTaken,
	}
	for repoErr, want := range sentinels {
		if got := mapCampaignError(repoErr); !errors.Is(got, want) {
			t.Errorf("%v mapped to %v, want %v", repoErr, got, want)
		}
	}

	wantValidationError(t, mapCampaignError(marketingrepo.ErrCampaignColumnReorderInvalid), "column_ids", "incomplete")
	wantValidationError(t, mapCampaignError(marketingrepo.ErrCampaignDateRange), "end_date", "before_start_date")
	wantValidationError(t, mapCampaignError(marketingrepo.ErrCampaignChannelInvalid), "channel", "oneof")

	cause := errors.New("insert refused")
	mapped := mapCampaignError(&marketingrepo.CampaignStageUnavailableError{Stage: "live", Cause: cause})
	var stageUnavailable *CampaignStageUnavailableError
	if !errors.As(mapped, &stageUnavailable) || stageUnavailable.Stage != "live" {
		t.Fatalf("stage error mapped to %v", mapped)
	}
	if !errors.Is(mapped, ErrCampaignStageUnavailable) || !errors.Is(mapped, cause) {
		t.Errorf("stage error must match its sentinel and keep its cause: %v", mapped)
	}

	unknown := errors.New("connection reset")
	if got := mapCampaignError(unknown); got != unknown {
		t.Errorf("an unknown error must pass through unchanged, got %v", got)
	}

	// The same mapping reaches the caller through the service.
	service := NewCampaignsService(&fakeCampaignsRepo{createErr: &marketingrepo.CampaignStageUnavailableError{Stage: "live"}}, nil, nil)
	if _, err := service.CreateCampaign(context.Background(), reportRequest(), "user-1"); !errors.Is(err, ErrCampaignStageUnavailable) {
		t.Errorf("create with an unavailable stage: %v", err)
	}
	service = NewCampaignsService(&fakeCampaignsRepo{updateErr: marketingrepo.ErrCampaignDateRange}, nil, nil)
	_, _, err := service.UpdateCampaign(context.Background(), "campaign-1", marketingdto.UpdateCampaignRequest(reportRequest()), "user-1")
	wantValidationError(t, err, "end_date", "before_start_date")
}
