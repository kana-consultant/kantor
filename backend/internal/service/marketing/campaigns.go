package marketing

import (
	"context"
	"errors"
	"fmt"
	"strings"

	marketingdto "github.com/kana-consultant/kantor/backend/internal/dto/marketing"
	"github.com/kana-consultant/kantor/backend/internal/model"
	authrepo "github.com/kana-consultant/kantor/backend/internal/repository/auth"
	marketingrepo "github.com/kana-consultant/kantor/backend/internal/repository/marketing"
	notificationsservice "github.com/kana-consultant/kantor/backend/internal/service/notifications"
)

var (
	ErrCampaignNotFound           = errors.New("campaign not found")
	ErrCampaignColumnNotFound     = errors.New("campaign column not found")
	ErrCampaignAttachmentNotFound = errors.New("campaign attachment not found")
	ErrCampaignPICNotFound        = errors.New("campaign pic employee not found")
	ErrCampaignColumnInUse        = errors.New("campaign column still has campaigns assigned")
)

type CampaignsService struct {
	repo                 *marketingrepo.CampaignsRepository
	authRepo             *authrepo.Repository
	notificationsService *notificationsservice.Service
}

type CampaignDetail struct {
	Campaign    model.Campaign             `json:"campaign"`
	Attachments []model.CampaignAttachment `json:"attachments"`
}

func NewCampaignsService(
	repo *marketingrepo.CampaignsRepository,
	authRepo *authrepo.Repository,
	notificationsService *notificationsservice.Service,
) *CampaignsService {
	return &CampaignsService{
		repo:                 repo,
		authRepo:             authRepo,
		notificationsService: notificationsService,
	}
}

func (s *CampaignsService) CreateCampaign(ctx context.Context, request marketingdto.CreateCampaignRequest, actorID string) (CampaignDetail, error) {
	item, err := s.repo.CreateCampaign(ctx, marketingrepo.UpsertCampaignParams{
		Name:           strings.TrimSpace(request.Name),
		Description:    trimOptionalString(request.Description),
		Channel:        request.Channel,
		BudgetAmount:   request.BudgetAmount,
		BudgetCurrency: normalizeCurrency(request.BudgetCurrency),
		PICEmployeeID:  trimOptionalString(request.PICEmployeeID),
		StartDate:      request.StartDate,
		EndDate:        request.EndDate,
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

func (s *CampaignsService) UpdateCampaign(ctx context.Context, campaignID string, request marketingdto.UpdateCampaignRequest, actorID string) (CampaignDetail, error) {
	item, err := s.repo.UpdateCampaign(ctx, campaignID, marketingrepo.UpsertCampaignParams{
		Name:           strings.TrimSpace(request.Name),
		Description:    trimOptionalString(request.Description),
		Channel:        request.Channel,
		BudgetAmount:   request.BudgetAmount,
		BudgetCurrency: normalizeCurrency(request.BudgetCurrency),
		PICEmployeeID:  trimOptionalString(request.PICEmployeeID),
		StartDate:      request.StartDate,
		EndDate:        request.EndDate,
		BriefText:      trimOptionalString(request.BriefText),
		Status:         request.Status,
		ActorID:        actorID,
	})
	if err != nil {
		return CampaignDetail{}, mapCampaignError(err)
	}

	return s.GetCampaign(ctx, item.ID)
}

func (s *CampaignsService) DeleteCampaign(ctx context.Context, campaignID string) error {
	return mapCampaignError(s.repo.DeleteCampaign(ctx, campaignID))
}

func (s *CampaignsService) ListKanban(ctx context.Context) ([]model.CampaignColumn, error) {
	items, err := s.repo.ListKanban(ctx)
	return items, mapCampaignError(err)
}

func (s *CampaignsService) MoveCampaign(ctx context.Context, campaignID string, request marketingdto.MoveCampaignRequest, actorID string) (CampaignDetail, error) {
	existing, err := s.repo.GetCampaignByID(ctx, campaignID)
	if err != nil {
		return CampaignDetail{}, mapCampaignError(err)
	}

	item, err := s.repo.MoveCampaign(ctx, campaignID, request.ColumnID, request.Position, actorID)
	if err != nil {
		return CampaignDetail{}, mapCampaignError(err)
	}

	if err := s.repo.LogActivity(ctx, item.ID, actorID, "campaign_moved", map[string]any{
		"from_status": existing.Status,
		"to_status":   item.Status,
		"column_id":   item.ColumnID,
		"column_name": valueOrFallback(item.ColumnName, "another stage"),
	}); err != nil {
		return CampaignDetail{}, err
	}

	if existing.Status != item.Status && item.Status == "live" {
		if notifyErr := s.notifyCampaignLive(ctx, item); notifyErr != nil {
			return CampaignDetail{}, notifyErr
		}
	}

	return s.GetCampaign(ctx, item.ID)
}

func (s *CampaignsService) AddAttachment(ctx context.Context, params marketingrepo.CreateCampaignAttachmentParams) (CampaignDetail, error) {
	attachment, err := s.repo.CreateAttachment(ctx, params)
	if err != nil {
		return CampaignDetail{}, mapCampaignError(err)
	}

	if err := s.repo.LogActivity(ctx, params.CampaignID, params.UploadedBy, "attachment_uploaded", map[string]any{
		"file_name": attachment.FileName,
		"file_type": attachment.FileType,
	}); err != nil {
		return CampaignDetail{}, err
	}
	return s.GetCampaign(ctx, params.CampaignID)
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

func (s *CampaignsService) CreateColumn(ctx context.Context, request marketingdto.CreateCampaignColumnRequest) (model.CampaignColumn, error) {
	item, err := s.repo.CreateColumn(ctx, marketingrepo.CreateCampaignColumnParams{
		Name:     strings.TrimSpace(request.Name),
		Color:    trimOptionalString(request.Color),
		Position: request.Position,
	})
	return item, mapCampaignError(err)
}

func (s *CampaignsService) UpdateColumn(ctx context.Context, columnID string, request marketingdto.UpdateCampaignColumnRequest) (model.CampaignColumn, error) {
	item, err := s.repo.UpdateColumn(ctx, columnID, marketingrepo.UpdateCampaignColumnParams{
		Name:  strings.TrimSpace(request.Name),
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
	switch {
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
	default:
		return err
	}
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

	managers, err := s.authRepo.ListUserIDsByRole(ctx, "manager", "marketing")
	if err != nil {
		return err
	}
	admins, err := s.authRepo.ListUserIDsByRole(ctx, "admin", "marketing")
	if err != nil {
		return err
	}
	superAdmins, err := s.authRepo.ListUserIDsByRole(ctx, "super_admin", "")
	if err != nil {
		return err
	}

	message := fmt.Sprintf("%s is now live and ready to monitor.", campaign.Name)
	return sendNotifications(
		ctx,
		s.notificationsService,
		append(append(append(managers, admins...), superAdmins...), campaign.CreatedBy),
		"marketing.campaign.live",
		"Campaign is now live",
		message,
		"campaign",
		&campaign.ID,
	)
}
