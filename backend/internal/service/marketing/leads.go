package marketing

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	marketingdto "github.com/kana-consultant/kantor/backend/internal/dto/marketing"
	"github.com/kana-consultant/kantor/backend/internal/model"
	marketingrepo "github.com/kana-consultant/kantor/backend/internal/repository/marketing"
	notificationsrepo "github.com/kana-consultant/kantor/backend/internal/repository/notifications"
)

var (
	ErrLeadNotFound             = errors.New("lead not found")
	ErrLeadAssignedUserNotFound = errors.New("lead assigned employee not found")
	ErrLeadCampaignNotFound     = errors.New("lead campaign not found")
	ErrLeadContactRequired      = errors.New("lead must include at least phone or email")
)

type leadsRepository interface {
	CreateLead(ctx context.Context, params marketingrepo.UpsertLeadParams) (model.Lead, error)
	ListLeads(ctx context.Context, params marketingrepo.ListLeadsParams) ([]model.Lead, int64, error)
	GetLeadByID(ctx context.Context, leadID string) (model.Lead, error)
	UpdateLead(ctx context.Context, leadID string, params marketingrepo.UpsertLeadParams) (model.Lead, error)
	DeleteLead(ctx context.Context, leadID string) error
	ListPipeline(ctx context.Context) ([]model.LeadPipelineColumn, error)
	MoveLeadStatus(ctx context.Context, leadID string, status string, actorID string) (model.Lead, error)
	ListActivities(ctx context.Context, leadID string) ([]model.LeadActivity, error)
	CreateActivity(ctx context.Context, params marketingrepo.CreateLeadActivityParams) (model.LeadActivity, error)
	Summary(ctx context.Context) (model.LeadSummary, error)
}

type leadsAuthRepository interface {
	ListUserIDsByPermission(ctx context.Context, permissionID string) ([]string, error)
}

type leadsNotificationsService interface {
	CreateMany(ctx context.Context, params []notificationsrepo.CreateParams) error
}

type LeadsService struct {
	repo                 leadsRepository
	authRepo             leadsAuthRepository
	notificationsService leadsNotificationsService
}

func NewLeadsService(
	repo leadsRepository,
	authRepo leadsAuthRepository,
	notificationsService leadsNotificationsService,
) *LeadsService {
	return &LeadsService{
		repo:                 repo,
		authRepo:             authRepo,
		notificationsService: notificationsService,
	}
}

func (s *LeadsService) CreateLead(ctx context.Context, request marketingdto.CreateLeadRequest, actorID string) (model.Lead, error) {
	params, err := s.buildUpsertParams(request, actorID)
	if err != nil {
		return model.Lead{}, err
	}

	item, repoErr := s.repo.CreateLead(ctx, params)
	return item, mapLeadError(repoErr)
}

func (s *LeadsService) ListLeads(ctx context.Context, query marketingdto.ListLeadsQuery) ([]model.Lead, int64, int, int, error) {
	page := query.Page
	if page <= 0 {
		page = 1
	}

	perPage := query.PerPage
	if perPage <= 0 {
		perPage = 20
	}

	items, total, err := s.repo.ListLeads(ctx, marketingrepo.ListLeadsParams{
		Page:           page,
		PerPage:        perPage,
		PipelineStatus: strings.TrimSpace(query.PipelineStatus),
		SourceChannel:  strings.TrimSpace(query.SourceChannel),
		CampaignID:     strings.TrimSpace(query.CampaignID),
		AssignedTo:     strings.TrimSpace(query.AssignedTo),
		DateFrom:       strings.TrimSpace(query.DateFrom),
		DateTo:         strings.TrimSpace(query.DateTo),
		Search:         strings.TrimSpace(query.Search),
	})
	return items, total, page, perPage, mapLeadError(err)
}

func (s *LeadsService) GetLead(ctx context.Context, leadID string) (model.Lead, error) {
	item, err := s.repo.GetLeadByID(ctx, strings.TrimSpace(leadID))
	return item, mapLeadError(err)
}

func (s *LeadsService) UpdateLead(ctx context.Context, leadID string, request marketingdto.UpdateLeadRequest, actorID string) (model.Lead, error) {
	params, err := s.buildUpsertParams(request, actorID)
	if err != nil {
		return model.Lead{}, err
	}

	item, repoErr := s.repo.UpdateLead(ctx, strings.TrimSpace(leadID), params)
	return item, mapLeadError(repoErr)
}

func (s *LeadsService) DeleteLead(ctx context.Context, leadID string) error {
	return mapLeadError(s.repo.DeleteLead(ctx, strings.TrimSpace(leadID)))
}

func (s *LeadsService) Pipeline(ctx context.Context) ([]model.LeadPipelineColumn, error) {
	items, err := s.repo.ListPipeline(ctx)
	return items, mapLeadError(err)
}

func (s *LeadsService) MoveStatus(ctx context.Context, leadID string, request marketingdto.MoveLeadStatusRequest, actorID string) (model.Lead, error) {
	existing, err := s.repo.GetLeadByID(ctx, strings.TrimSpace(leadID))
	if err != nil {
		return model.Lead{}, mapLeadError(err)
	}

	item, err := s.repo.MoveLeadStatus(ctx, strings.TrimSpace(leadID), strings.TrimSpace(request.PipelineStatus), actorID)
	if err != nil {
		return model.Lead{}, mapLeadError(err)
	}

	if existing.PipelineStatus != item.PipelineStatus && (item.PipelineStatus == "won" || item.PipelineStatus == "lost") {
		if notifyErr := s.notifyLeadOutcome(ctx, item); notifyErr != nil {
			return model.Lead{}, notifyErr
		}
	}

	return item, nil
}

func (s *LeadsService) ListActivities(ctx context.Context, leadID string) ([]model.LeadActivity, error) {
	items, err := s.repo.ListActivities(ctx, strings.TrimSpace(leadID))
	return items, mapLeadError(err)
}

func (s *LeadsService) CreateActivity(ctx context.Context, leadID string, request marketingdto.CreateLeadActivityRequest, actorID string) (model.LeadActivity, error) {
	item, err := s.repo.CreateActivity(ctx, marketingrepo.CreateLeadActivityParams{
		LeadID:       strings.TrimSpace(leadID),
		ActivityType: strings.TrimSpace(request.ActivityType),
		Description:  strings.TrimSpace(request.Description),
		CreatedBy:    actorID,
	})
	return item, mapLeadError(err)
}

func (s *LeadsService) ImportCSV(ctx context.Context, reader io.Reader, actorID string) (model.LeadImportSummary, error) {
	csvReader := csv.NewReader(reader)
	csvReader.TrimLeadingSpace = true

	rows, err := csvReader.ReadAll()
	if err != nil {
		return model.LeadImportSummary{}, err
	}
	if len(rows) <= 1 {
		return model.LeadImportSummary{}, nil
	}

	summary := model.LeadImportSummary{
		Errors: make([]model.LeadImportError, 0),
	}

	for index, row := range rows[1:] {
		lineNumber := index + 2
		request, parseErr := parseLeadCSVRow(row)
		if parseErr != nil {
			summary.FailedCount++
			summary.Errors = append(summary.Errors, model.LeadImportError{
				Row:     lineNumber,
				Message: parseErr.Error(),
			})
			continue
		}

		if _, createErr := s.CreateLead(ctx, request, actorID); createErr != nil {
			summary.FailedCount++
			summary.Errors = append(summary.Errors, model.LeadImportError{
				Row:     lineNumber,
				Message: createErr.Error(),
			})
			continue
		}

		summary.SuccessCount++
	}

	return summary, nil
}

func (s *LeadsService) Summary(ctx context.Context) (model.LeadSummary, error) {
	item, err := s.repo.Summary(ctx)
	return item, mapLeadError(err)
}

func (s *LeadsService) buildUpsertParams(request marketingdto.CreateLeadRequest, actorID string) (marketingrepo.UpsertLeadParams, error) {
	phone := trimOptionalString(request.Phone)
	email := trimOptionalString(request.Email)
	if phone == nil && email == nil {
		return marketingrepo.UpsertLeadParams{}, ErrLeadContactRequired
	}

	return marketingrepo.UpsertLeadParams{
		Name:           strings.TrimSpace(request.Name),
		Phone:          phone,
		Email:          email,
		SourceChannel:  strings.TrimSpace(request.SourceChannel),
		PipelineStatus: strings.TrimSpace(request.PipelineStatus),
		CampaignID:     trimOptionalString(request.CampaignID),
		AssignedTo:     trimOptionalString(request.AssignedTo),
		Notes:          trimOptionalString(request.Notes),
		CompanyName:    trimOptionalString(request.CompanyName),
		EstimatedValue: request.EstimatedValue,
		CreatedBy:      actorID,
	}, nil
}

func mapLeadError(err error) error {
	switch {
	case errors.Is(err, marketingrepo.ErrLeadNotFound):
		return ErrLeadNotFound
	case errors.Is(err, marketingrepo.ErrLeadAssignedUserMissing):
		return ErrLeadAssignedUserNotFound
	case errors.Is(err, marketingrepo.ErrLeadCampaignMissing):
		return ErrLeadCampaignNotFound
	default:
		return err
	}
}

func parseLeadCSVRow(row []string) (marketingdto.CreateLeadRequest, error) {
	columns := make([]string, 9)
	copy(columns, row)

	estimatedValue := int64(0)
	if value := strings.TrimSpace(columns[8]); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return marketingdto.CreateLeadRequest{}, errors.New("estimated_value must be a number")
		}
		estimatedValue = parsed
	}

	return marketingdto.CreateLeadRequest{
		Name:           strings.TrimSpace(columns[0]),
		Phone:          stringPointer(columns[1]),
		Email:          stringPointer(columns[2]),
		SourceChannel:  strings.TrimSpace(columns[3]),
		PipelineStatus: strings.TrimSpace(columns[4]),
		CampaignID:     nil,
		AssignedTo:     stringPointer(columns[5]),
		Notes:          stringPointer(columns[6]),
		CompanyName:    stringPointer(columns[7]),
		EstimatedValue: estimatedValue,
	}, nil
}

func stringPointer(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func (s *LeadsService) notifyLeadOutcome(ctx context.Context, lead model.Lead) error {
	if s.authRepo == nil || s.notificationsService == nil {
		return nil
	}

	recipients, err := s.authRepo.ListUserIDsByPermission(ctx, "marketing:leads:edit")
	if err != nil {
		return err
	}

	message := fmt.Sprintf("%s moved to %s.", lead.Name, strings.ToUpper(lead.PipelineStatus))
	return sendNotifications(
		ctx,
		s.notificationsService,
		append(recipients, lead.CreatedBy),
		"marketing.lead."+lead.PipelineStatus,
		"Lead outcome updated",
		message,
		"lead",
		&lead.ID,
	)
}
