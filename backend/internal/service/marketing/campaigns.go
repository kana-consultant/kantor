package marketing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	shareddto "github.com/kana-consultant/kantor/backend/internal/dto"
	marketingdto "github.com/kana-consultant/kantor/backend/internal/dto/marketing"
	"github.com/kana-consultant/kantor/backend/internal/model"
	"github.com/kana-consultant/kantor/backend/internal/repository"
	marketingrepo "github.com/kana-consultant/kantor/backend/internal/repository/marketing"
	notificationsrepo "github.com/kana-consultant/kantor/backend/internal/repository/notifications"
)

var (
	ErrCampaignNotFound           = errors.New("campaign not found")
	ErrCampaignColumnNotFound     = errors.New("campaign column not found")
	ErrCampaignAttachmentNotFound = errors.New("campaign attachment not found")
	ErrCampaignPICNotFound        = errors.New("campaign pic employee not found")
	ErrCampaignColumnInUse        = errors.New("campaign column still has campaigns assigned")
	ErrCampaignColumnProtected    = errors.New("this column is the lane of a campaign stage and cannot be deleted; rename it instead")
	ErrCampaignColumnNameTaken    = errors.New("another campaign column already has this name")
	// ErrCampaignStageUnavailable is matched by *CampaignStageUnavailableError.
	ErrCampaignStageUnavailable = errors.New("campaign stage has no board column")
)

// CampaignValidationError is a request the validator let through but the
// campaign rules reject. Field is the JSON field it belongs to and Rule the
// short reason, both returned in the error details.
type CampaignValidationError struct {
	Field   string
	Rule    string
	Message string
}

func (e *CampaignValidationError) Error() string { return e.Message }

// CampaignStageUnavailableError: the board has no column for Stage, and
// creating it on the spot did not work either (Cause, when known).
type CampaignStageUnavailableError struct {
	Stage string
	Cause error
}

func (e *CampaignStageUnavailableError) Error() string {
	return fmt.Sprintf("the campaign board has no column for the stage %q", e.Stage)
}

func (e *CampaignStageUnavailableError) Is(target error) bool {
	return target == ErrCampaignStageUnavailable
}

func (e *CampaignStageUnavailableError) Unwrap() error { return e.Cause }

const (
	campaignNameMinLength       = 3
	campaignColumnNameMinLength = 2
)

type campaignsRepository interface {
	BeginTx(ctx context.Context) (pgx.Tx, error)
	CreateCampaign(ctx context.Context, params marketingrepo.UpsertCampaignParams) (model.Campaign, error)
	ListCampaigns(ctx context.Context, params marketingrepo.ListCampaignsParams) ([]model.Campaign, int64, error)
	GetCampaignByID(ctx context.Context, campaignID string) (model.Campaign, error)
	UpdateCampaign(ctx context.Context, campaignID string, params marketingrepo.UpsertCampaignParams) (model.Campaign, string, error)
	DeleteCampaign(ctx context.Context, campaignID string) error
	ListKanban(ctx context.Context) ([]model.CampaignColumn, error)
	MoveCampaign(ctx context.Context, campaignID string, columnID string, position int, movedBy string) (model.Campaign, error)
	ListColumns(ctx context.Context) ([]model.CampaignColumn, error)
	ListPICOptions(ctx context.Context) ([]model.CampaignPICOption, error)
	CreateColumn(ctx context.Context, params marketingrepo.CreateCampaignColumnParams) (model.CampaignColumn, error)
	UpdateColumn(ctx context.Context, columnID string, params marketingrepo.UpdateCampaignColumnParams) (model.CampaignColumn, error)
	DeleteColumn(ctx context.Context, columnID string) error
	ReorderColumns(ctx context.Context, columnIDs []string) error
	CreateAttachment(ctx context.Context, params marketingrepo.CreateCampaignAttachmentParams) (model.CampaignAttachment, error)
	ListAttachments(ctx context.Context, campaignID string) ([]model.CampaignAttachment, error)
	DeleteAttachment(ctx context.Context, campaignID string, attachmentID string) (model.CampaignAttachment, error)
	ListActivities(ctx context.Context, campaignID string) ([]model.CampaignActivity, error)
	LogActivity(ctx context.Context, campaignID string, actorID string, action string, payload map[string]any) error
}

type campaignsAuthRepository interface {
	ListUserIDsByPermission(ctx context.Context, permissionID string) ([]string, error)
}

type campaignsNotificationsService interface {
	CreateMany(ctx context.Context, params []notificationsrepo.CreateParams) error
}

type CampaignsService struct {
	repo                 campaignsRepository
	authRepo             campaignsAuthRepository
	notificationsService campaignsNotificationsService
}

type CampaignDetail struct {
	Campaign    model.Campaign             `json:"campaign"`
	Attachments []model.CampaignAttachment `json:"attachments"`
}

func NewCampaignsService(
	repo campaignsRepository,
	authRepo campaignsAuthRepository,
	notificationsService campaignsNotificationsService,
) *CampaignsService {
	return &CampaignsService{
		repo:                 repo,
		authRepo:             authRepo,
		notificationsService: notificationsService,
	}
}

func (s *CampaignsService) CreateCampaign(ctx context.Context, request marketingdto.CreateCampaignRequest, actorID string) (CampaignDetail, error) {
	name, startDate, endDate, err := normalizeCampaignInput(request.Name, request.StartDate, request.EndDate)
	if err != nil {
		return CampaignDetail{}, err
	}

	item, err := s.repo.CreateCampaign(ctx, marketingrepo.UpsertCampaignParams{
		Name:           name,
		Description:    trimOptionalString(request.Description),
		Channel:        request.Channel,
		BudgetAmount:   request.BudgetAmount,
		BudgetCurrency: normalizeCurrency(request.BudgetCurrency),
		PICEmployeeID:  trimOptionalString(request.PICEmployeeID),
		StartDate:      startDate,
		EndDate:        endDate,
		BriefText:      trimOptionalString(request.BriefText),
		Status:         request.Status,
		ActorID:        actorID,
	})
	if err != nil {
		return CampaignDetail{}, mapCampaignError(err)
	}
	return s.GetCampaign(ctx, item.ID)
}

func (s *CampaignsService) ListCampaigns(ctx context.Context, query marketingdto.ListCampaignsQuery) ([]model.Campaign, int64, int, int, error) {
	page := query.Page
	if page <= 0 {
		page = 1
	}

	perPage := query.PerPage
	if perPage <= 0 {
		perPage = 12
	}

	items, total, err := s.repo.ListCampaigns(ctx, marketingrepo.ListCampaignsParams{
		Page:     page,
		PerPage:  perPage,
		Search:   strings.TrimSpace(query.Search),
		Channel:  strings.TrimSpace(query.Channel),
		Status:   strings.TrimSpace(query.Status),
		PIC:      strings.TrimSpace(query.PIC),
		DateFrom: strings.TrimSpace(query.DateFrom),
		DateTo:   strings.TrimSpace(query.DateTo),
	})
	if err != nil {
		return nil, 0, 0, 0, err
	}

	return items, total, page, perPage, nil
}

func (s *CampaignsService) GetCampaign(ctx context.Context, campaignID string) (CampaignDetail, error) {
	item, err := s.repo.GetCampaignByID(ctx, campaignID)
	if err != nil {
		return CampaignDetail{}, mapCampaignError(err)
	}

	attachments, err := s.repo.ListAttachments(ctx, campaignID)
	if err != nil {
		return CampaignDetail{}, mapCampaignError(err)
	}

	return CampaignDetail{
		Campaign:    item,
		Attachments: attachments,
	}, nil
}

// UpdateCampaign saves the campaign. The second return value is the stage
// the campaign had before; it differs from the returned campaign's status
// exactly when this update moved the card to another stage lane.
func (s *CampaignsService) UpdateCampaign(ctx context.Context, campaignID string, request marketingdto.UpdateCampaignRequest, actorID string) (CampaignDetail, string, error) {
	name, startDate, endDate, err := normalizeCampaignInput(request.Name, request.StartDate, request.EndDate)
	if err != nil {
		return CampaignDetail{}, "", err
	}

	item, previousStatus, err := s.repo.UpdateCampaign(ctx, campaignID, marketingrepo.UpsertCampaignParams{
		Name:           name,
		Description:    trimOptionalString(request.Description),
		Channel:        request.Channel,
		BudgetAmount:   request.BudgetAmount,
		BudgetCurrency: normalizeCurrency(request.BudgetCurrency),
		PICEmployeeID:  trimOptionalString(request.PICEmployeeID),
		StartDate:      startDate,
		EndDate:        endDate,
		BriefText:      trimOptionalString(request.BriefText),
		Status:         request.Status,
		ActorID:        actorID,
	})
	if err != nil {
		return CampaignDetail{}, "", mapCampaignError(err)
	}

	detail, err := s.GetCampaign(ctx, item.ID)
	return detail, previousStatus, err
}

func (s *CampaignsService) DeleteCampaign(ctx context.Context, campaignID string) error {
	return mapCampaignError(s.repo.DeleteCampaign(ctx, campaignID))
}

func (s *CampaignsService) ListKanban(ctx context.Context) ([]model.CampaignColumn, error) {
	items, err := s.repo.ListKanban(ctx)
	return items, mapCampaignError(err)
}

// MoveCampaign moves the card and then records the move and, when the
// campaign just went live, notifies the campaign editors. Both follow-ups
// happen after the move is committed and are best effort: a failure there is
// logged and the moved campaign is still returned, because the move itself
// succeeded and reporting an error would make the client undo a change that
// is already stored.
func (s *CampaignsService) MoveCampaign(ctx context.Context, campaignID string, request marketingdto.MoveCampaignRequest, actorID string) (CampaignDetail, error) {
	existing, err := s.repo.GetCampaignByID(ctx, campaignID)
	if err != nil {
		return CampaignDetail{}, mapCampaignError(err)
	}

	item, err := s.repo.MoveCampaign(ctx, campaignID, request.ColumnID, request.Position, actorID)
	if err != nil {
		return CampaignDetail{}, mapCampaignError(err)
	}

	if logErr := s.repo.LogActivity(ctx, item.ID, actorID, "campaign_moved", map[string]any{
		"from_status": existing.Status,
		"to_status":   item.Status,
		"column_id":   item.ColumnID,
		"column_name": valueOrFallback(item.ColumnName, "another stage"),
	}); logErr != nil {
		slog.WarnContext(ctx, "campaign moved but the activity entry was not written", "campaign_id", item.ID, "error", logErr)
	}

	if existing.Status != item.Status && item.Status == "live" {
		if notifyErr := s.notifyCampaignLive(ctx, item); notifyErr != nil {
			slog.WarnContext(ctx, "campaign moved to live but the notification was not sent", "campaign_id", item.ID, "error", notifyErr)
		}
	}

	return s.GetCampaign(ctx, item.ID)
}

func (s *CampaignsService) AddAttachment(ctx context.Context, params marketingrepo.CreateCampaignAttachmentParams) (CampaignDetail, error) {
	return s.AddAttachments(ctx, []marketingrepo.CreateCampaignAttachmentParams{params})
}

func (s *CampaignsService) AddAttachments(ctx context.Context, params []marketingrepo.CreateCampaignAttachmentParams) (CampaignDetail, error) {
	if len(params) == 0 {
		return CampaignDetail{}, fmt.Errorf("at least one attachment is required")
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return CampaignDetail{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	txCtx := repository.WithConn(ctx, tx)
	campaignID := params[0].CampaignID

	for _, item := range params {
		if item.CampaignID != campaignID {
			return CampaignDetail{}, fmt.Errorf("all attachments must target the same campaign")
		}

		attachment, err := s.repo.CreateAttachment(txCtx, item)
		if err != nil {
			return CampaignDetail{}, mapCampaignError(err)
		}

		if err := s.repo.LogActivity(txCtx, item.CampaignID, item.UploadedBy, "attachment_uploaded", map[string]any{
			"file_name": attachment.FileName,
			"file_type": attachment.FileType,
		}); err != nil {
			return CampaignDetail{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return CampaignDetail{}, err
	}

	return s.GetCampaign(ctx, campaignID)
}

func (s *CampaignsService) ListAttachments(ctx context.Context, campaignID string) ([]model.CampaignAttachment, error) {
	items, err := s.repo.ListAttachments(ctx, campaignID)
	return items, mapCampaignError(err)
}

func (s *CampaignsService) DeleteAttachment(ctx context.Context, campaignID string, attachmentID string) (model.CampaignAttachment, error) {
	item, err := s.repo.DeleteAttachment(ctx, campaignID, attachmentID)
	return item, mapCampaignError(err)
}

func (s *CampaignsService) ListActivities(ctx context.Context, campaignID string) ([]model.CampaignActivity, error) {
	items, err := s.repo.ListActivities(ctx, campaignID)
	return items, mapCampaignError(err)
}

func (s *CampaignsService) ListColumns(ctx context.Context) ([]model.CampaignColumn, error) {
	items, err := s.repo.ListColumns(ctx)
	return items, mapCampaignError(err)
}

// ListPICOptions lists the employees that can be put in charge of a campaign.
func (s *CampaignsService) ListPICOptions(ctx context.Context) ([]model.CampaignPICOption, error) {
	return s.repo.ListPICOptions(ctx)
}

func (s *CampaignsService) CreateColumn(ctx context.Context, request marketingdto.CreateCampaignColumnRequest) (model.CampaignColumn, error) {
	name, err := normalizeCampaignColumnName(request.Name)
	if err != nil {
		return model.CampaignColumn{}, err
	}
	item, err := s.repo.CreateColumn(ctx, marketingrepo.CreateCampaignColumnParams{
		Name:     name,
		Color:    trimOptionalString(request.Color),
		Position: request.Position,
	})
	return item, mapCampaignError(err)
}

func (s *CampaignsService) UpdateColumn(ctx context.Context, columnID string, request marketingdto.UpdateCampaignColumnRequest) (model.CampaignColumn, error) {
	name, err := normalizeCampaignColumnName(request.Name)
	if err != nil {
		return model.CampaignColumn{}, err
	}
	item, err := s.repo.UpdateColumn(ctx, columnID, marketingrepo.UpdateCampaignColumnParams{
		Name:  name,
		Color: trimOptionalString(request.Color),
	})
	return item, mapCampaignError(err)
}

func (s *CampaignsService) DeleteColumn(ctx context.Context, columnID string) error {
	return mapCampaignError(s.repo.DeleteColumn(ctx, columnID))
}

func (s *CampaignsService) ReorderColumns(ctx context.Context, request marketingdto.ReorderCampaignColumnsRequest) error {
	return mapCampaignError(s.repo.ReorderColumns(ctx, request.ColumnIDs))
}

func mapCampaignError(err error) error {
	var stageUnavailable *marketingrepo.CampaignStageUnavailableError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &stageUnavailable):
		return &CampaignStageUnavailableError{Stage: stageUnavailable.Stage, Cause: stageUnavailable.Cause}
	case errors.Is(err, marketingrepo.ErrCampaignNotFound):
		return ErrCampaignNotFound
	case errors.Is(err, marketingrepo.ErrCampaignColumnNotFound):
		return ErrCampaignColumnNotFound
	case errors.Is(err, marketingrepo.ErrCampaignAttachmentNotFound):
		return ErrCampaignAttachmentNotFound
	case errors.Is(err, marketingrepo.ErrCampaignPICNotFound):
		return ErrCampaignPICNotFound
	case errors.Is(err, marketingrepo.ErrCampaignColumnInUse):
		return ErrCampaignColumnInUse
	case errors.Is(err, marketingrepo.ErrCampaignColumnProtected):
		return ErrCampaignColumnProtected
	case errors.Is(err, marketingrepo.ErrCampaignColumnNameTaken):
		return ErrCampaignColumnNameTaken
	case errors.Is(err, marketingrepo.ErrCampaignColumnReorderInvalid):
		return &CampaignValidationError{Field: "column_ids", Rule: "incomplete", Message: "column_ids must list every campaign column exactly once"}
	case errors.Is(err, marketingrepo.ErrCampaignDateRange):
		return errCampaignEndBeforeStart()
	case errors.Is(err, marketingrepo.ErrCampaignChannelInvalid):
		return &CampaignValidationError{Field: "channel", Rule: "oneof", Message: "channel is not one of the allowed channels"}
	default:
		return err
	}
}

func errCampaignEndBeforeStart() error {
	return &CampaignValidationError{Field: "end_date", Rule: "before_start_date", Message: "end_date must not be before start_date"}
}

// normalizeCampaignInput applies the rules the struct validator cannot: the
// name is measured after trimming (a name of spaces is not a name) and the
// end date must not precede the start date.
func normalizeCampaignInput(rawName string, rawStart shareddto.DateOnly, rawEnd shareddto.DateOnly) (string, time.Time, time.Time, error) {
	name := strings.TrimSpace(rawName)
	if utf8.RuneCountInString(name) < campaignNameMinLength {
		return "", time.Time{}, time.Time{}, &CampaignValidationError{
			Field:   "name",
			Rule:    "min",
			Message: fmt.Sprintf("name must have at least %d characters", campaignNameMinLength),
		}
	}

	startDate, err := shareddto.ParseDateOnly(rawStart)
	if err != nil {
		return "", time.Time{}, time.Time{}, &CampaignValidationError{Field: "start_date", Rule: "datetime", Message: "start_date must be a date (YYYY-MM-DD)"}
	}
	endDate, err := shareddto.ParseDateOnly(rawEnd)
	if err != nil {
		return "", time.Time{}, time.Time{}, &CampaignValidationError{Field: "end_date", Rule: "datetime", Message: "end_date must be a date (YYYY-MM-DD)"}
	}
	if endDate.Before(startDate) {
		return "", time.Time{}, time.Time{}, errCampaignEndBeforeStart()
	}

	return name, startDate, endDate, nil
}

func normalizeCampaignColumnName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if utf8.RuneCountInString(name) < campaignColumnNameMinLength {
		return "", &CampaignValidationError{
			Field:   "name",
			Rule:    "min",
			Message: fmt.Sprintf("name must have at least %d characters", campaignColumnNameMinLength),
		}
	}
	return name, nil
}

func trimOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}

func normalizeCurrency(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "IDR"
	}
	return strings.ToUpper(trimmed)
}

func valueOrFallback(value *string, fallback string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return fallback
	}
	return strings.TrimSpace(*value)
}

func (s *CampaignsService) notifyCampaignLive(ctx context.Context, campaign model.Campaign) error {
	if s.authRepo == nil || s.notificationsService == nil {
		return nil
	}

	recipients, err := s.authRepo.ListUserIDsByPermission(ctx, "marketing:campaign:edit")
	if err != nil {
		return err
	}

	message := fmt.Sprintf("%s is now live and ready to monitor.", campaign.Name)
	return sendNotifications(
		ctx,
		s.notificationsService,
		append(recipients, campaign.CreatedBy),
		"marketing.campaign.live",
		"Campaign is now live",
		message,
		"campaign",
		&campaign.ID,
	)
}
