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
	ErrLeadImportLimitExceeded  = errors.New("lead import exceeds 10000 rows")
	ErrLeadImportMissingHeader  = errors.New("lead CSV is missing required header columns")
)

// leadCSVRequiredHeaders are the header columns the importer needs to find in
// the first row of the CSV. The importer is header-driven (not positional) so
// callers can produce CSVs from any tool as long as the columns are named.
var leadCSVRequiredHeaders = []string{"name", "source_channel", "pipeline_status"}

// leadCSVKnownHeaders is the full set of columns the importer recognises.
// Anything not in this list is ignored without an error.
var leadCSVKnownHeaders = []string{
	"name",
	"phone",
	"email",
	"source_channel",
	"pipeline_status",
	"assigned_to",
	"notes",
	"company_name",
	"estimated_value",
}

const maxLeadImportRows = 10000

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
	csvReader.FieldsPerRecord = -1

	header, err := csvReader.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return model.LeadImportSummary{}, nil
		}
		return model.LeadImportSummary{}, err
	}
	if len(header) == 0 {
		return model.LeadImportSummary{}, nil
	}

	headerIndex, missing := buildLeadCSVHeaderIndex(header)
	if len(missing) > 0 {
		return model.LeadImportSummary{}, fmt.Errorf("%w: %s", ErrLeadImportMissingHeader, strings.Join(missing, ", "))
	}

	summary := model.LeadImportSummary{
		Errors: make([]model.LeadImportError, 0),
	}

	importedRows := 0
	lineNumber := 1
	for {
		row, readErr := csvReader.Read()
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return model.LeadImportSummary{}, readErr
		}

		lineNumber++
		if isBlankLeadCSVRow(row) {
			continue
		}

		importedRows++
		if importedRows > maxLeadImportRows {
			return summary, ErrLeadImportLimitExceeded
		}

		request, parseErr := parseLeadCSVRow(row, headerIndex)
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

// buildLeadCSVHeaderIndex normalises the header row (lower-case, trimmed,
// underscores instead of spaces/dashes) and returns a map from canonical
// header name to column index. Unknown columns are kept in the map so the
// caller can decide whether to ignore them. Required headers that are not
// present are returned in `missing`.
func buildLeadCSVHeaderIndex(header []string) (map[string]int, []string) {
	known := make(map[string]struct{}, len(leadCSVKnownHeaders))
	for _, name := range leadCSVKnownHeaders {
		known[name] = struct{}{}
	}

	index := make(map[string]int, len(header))
	for i, raw := range header {
		key := normaliseLeadCSVHeader(raw)
		if key == "" {
			continue
		}
		if _, ok := known[key]; !ok {
			continue
		}
		if _, exists := index[key]; exists {
			continue
		}
		index[key] = i
	}

	missing := make([]string, 0, len(leadCSVRequiredHeaders))
	for _, required := range leadCSVRequiredHeaders {
		if _, ok := index[required]; !ok {
			missing = append(missing, required)
		}
	}
	return index, missing
}

func normaliseLeadCSVHeader(value string) string {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	trimmed = strings.ReplaceAll(trimmed, "-", "_")
	trimmed = strings.ReplaceAll(trimmed, " ", "_")
	for strings.Contains(trimmed, "__") {
		trimmed = strings.ReplaceAll(trimmed, "__", "_")
	}
	return trimmed
}

func isBlankLeadCSVRow(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
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

func parseLeadCSVRow(row []string, headerIndex map[string]int) (marketingdto.CreateLeadRequest, error) {
	get := func(key string) string {
		idx, ok := headerIndex[key]
		if !ok || idx < 0 || idx >= len(row) {
			return ""
		}
		return row[idx]
	}

	estimatedValue := int64(0)
	if value := strings.TrimSpace(get("estimated_value")); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return marketingdto.CreateLeadRequest{}, errors.New("estimated_value must be a number")
		}
		estimatedValue = parsed
	}

	return marketingdto.CreateLeadRequest{
		Name:           strings.TrimSpace(get("name")),
		Phone:          stringPointer(get("phone")),
		Email:          stringPointer(get("email")),
		SourceChannel:  strings.TrimSpace(get("source_channel")),
		PipelineStatus: strings.TrimSpace(get("pipeline_status")),
		CampaignID:     nil,
		AssignedTo:     stringPointer(get("assigned_to")),
		Notes:          stringPointer(get("notes")),
		CompanyName:    stringPointer(get("company_name")),
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
