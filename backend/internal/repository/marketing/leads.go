package marketing

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kana-consultant/kantor/backend/internal/model"
)

var (
	ErrLeadNotFound            = errors.New("lead not found")
	ErrLeadAssignedUserMissing = errors.New("lead assigned employee not found")
	ErrLeadCampaignMissing     = errors.New("lead campaign not found")
)

type LeadsRepository struct {
	db *pgxpool.Pool
}

type UpsertLeadParams struct {
	Name           string
	Phone          *string
	Email          *string
	SourceChannel  string
	PipelineStatus string
	CampaignID     *string
	AssignedTo     *string
	Notes          *string
	CompanyName    *string
	EstimatedValue int64
	CreatedBy      string
}

type ListLeadsParams struct {
	Page           int
	PerPage        int
	PipelineStatus string
	SourceChannel  string
	CampaignID     string
	AssignedTo     string
	DateFrom       string
	DateTo         string
	Search         string
}

type CreateLeadActivityParams struct {
	LeadID       string
	ActivityType string
	Description  string
	OldStatus    *string
	NewStatus    *string
	CreatedBy    string
}

func NewLeadsRepository(db *pgxpool.Pool) *LeadsRepository {
	return &LeadsRepository{db: db}
}

func (r *LeadsRepository) CreateLead(ctx context.Context, params UpsertLeadParams) (model.Lead, error) {
	if err := r.ensureEmployeeExists(ctx, params.AssignedTo); err != nil {
		return model.Lead{}, err
	}
	if err := r.ensureCampaignExists(ctx, params.CampaignID); err != nil {
		return model.Lead{}, err
	}

	row := r.db.QueryRow(ctx, `
		INSERT INTO leads (
			name, phone, email, source_channel, pipeline_status, campaign_id, assigned_to, notes, company_name, estimated_value, created_by
		)
		VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), $4, $5, NULLIF($6, '')::uuid, NULLIF($7, '')::uuid, NULLIF($8, ''), NULLIF($9, ''), $10, $11::uuid)
		RETURNING id::text
	`,
		params.Name,
		nullableLeadText(params.Phone),
		nullableLeadText(params.Email),
		params.SourceChannel,
		params.PipelineStatus,
		nullableLeadText(params.CampaignID),
		nullableLeadText(params.AssignedTo),
		nullableLeadText(params.Notes),
		nullableLeadText(params.CompanyName),
		params.EstimatedValue,
		params.CreatedBy,
	)

	var leadID string
	if err := row.Scan(&leadID); err != nil {
		return model.Lead{}, err
	}

	return r.GetLeadByID(ctx, leadID)
}

func (r *LeadsRepository) ListLeads(ctx context.Context, params ListLeadsParams) ([]model.Lead, int64, error) {
	filters := []string{"1=1"}
	args := make([]interface{}, 0)
	index := 1

	if status := strings.TrimSpace(params.PipelineStatus); status != "" {
		filters = append(filters, fmt.Sprintf("leads.pipeline_status = $%d", index))
		args = append(args, status)
		index++
	}
	if source := strings.TrimSpace(params.SourceChannel); source != "" {
		filters = append(filters, fmt.Sprintf("leads.source_channel = $%d", index))
		args = append(args, source)
		index++
	}
	if campaignID := strings.TrimSpace(params.CampaignID); campaignID != "" {
		filters = append(filters, fmt.Sprintf("leads.campaign_id = $%d::uuid", index))
		args = append(args, campaignID)
		index++
	}
	if assignedTo := strings.TrimSpace(params.AssignedTo); assignedTo != "" {
		filters = append(filters, fmt.Sprintf("leads.assigned_to = $%d::uuid", index))
		args = append(args, assignedTo)
		index++
	}
	if dateFrom := strings.TrimSpace(params.DateFrom); dateFrom != "" {
		filters = append(filters, fmt.Sprintf("leads.created_at::date >= $%d::date", index))
		args = append(args, dateFrom)
		index++
	}
	if dateTo := strings.TrimSpace(params.DateTo); dateTo != "" {
		filters = append(filters, fmt.Sprintf("leads.created_at::date <= $%d::date", index))
		args = append(args, dateTo)
		index++
	}
	if search := strings.TrimSpace(params.Search); search != "" {
		filters = append(filters, fmt.Sprintf("(leads.name ILIKE $%d OR COALESCE(leads.phone, '') ILIKE $%d OR COALESCE(leads.email, '') ILIKE $%d)", index, index, index))
		args = append(args, "%"+search+"%")
		index++
	}

	whereClause := strings.Join(filters, " AND ")

	var total int64
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM leads WHERE `+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.PerPage
	query := fmt.Sprintf(`
		SELECT
			leads.id::text,
			leads.name,
			leads.phone,
			leads.email,
			leads.source_channel,
			leads.pipeline_status,
			leads.campaign_id::text,
			campaigns.name,
			leads.assigned_to::text,
			employees.full_name,
			employees.avatar_url,
			leads.notes,
			leads.company_name,
			leads.estimated_value,
			leads.created_by::text,
			users.full_name,
			leads.created_at,
			leads.updated_at
		FROM leads
		LEFT JOIN campaigns ON campaigns.id = leads.campaign_id
		LEFT JOIN employees ON employees.id = leads.assigned_to
		LEFT JOIN users ON users.id = leads.created_by
		WHERE %s
		ORDER BY leads.updated_at DESC, leads.created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, index, index+1)
	args = append(args, params.PerPage, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]model.Lead, 0)
	for rows.Next() {
		var item model.Lead
		if err := scanLead(rows, &item); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}

	return items, total, rows.Err()
}

func (r *LeadsRepository) GetLeadByID(ctx context.Context, leadID string) (model.Lead, error) {
	row := r.db.QueryRow(ctx, `
		SELECT
			leads.id::text,
			leads.name,
			leads.phone,
			leads.email,
			leads.source_channel,
			leads.pipeline_status,
			leads.campaign_id::text,
			campaigns.name,
			leads.assigned_to::text,
			employees.full_name,
			employees.avatar_url,
			leads.notes,
			leads.company_name,
			leads.estimated_value,
			leads.created_by::text,
			users.full_name,
			leads.created_at,
			leads.updated_at
		FROM leads
		LEFT JOIN campaigns ON campaigns.id = leads.campaign_id
		LEFT JOIN employees ON employees.id = leads.assigned_to
		LEFT JOIN users ON users.id = leads.created_by
		WHERE leads.id = $1::uuid
	`, leadID)

	var item model.Lead
	if err := scanLeadRow(row, &item); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Lead{}, ErrLeadNotFound
		}
		return model.Lead{}, err
	}

	return item, nil
}

func (r *LeadsRepository) UpdateLead(ctx context.Context, leadID string, params UpsertLeadParams) (model.Lead, error) {
	if err := r.ensureEmployeeExists(ctx, params.AssignedTo); err != nil {
		return model.Lead{}, err
	}
	if err := r.ensureCampaignExists(ctx, params.CampaignID); err != nil {
		return model.Lead{}, err
	}

	tag, err := r.db.Exec(ctx, `
		UPDATE leads
		SET
			name = $2,
			phone = NULLIF($3, ''),
			email = NULLIF($4, ''),
			source_channel = $5,
			pipeline_status = $6,
			campaign_id = NULLIF($7, '')::uuid,
			assigned_to = NULLIF($8, '')::uuid,
			notes = NULLIF($9, ''),
			company_name = NULLIF($10, ''),
			estimated_value = $11,
			updated_at = NOW()
		WHERE id = $1::uuid
	`,
		leadID,
		params.Name,
		nullableLeadText(params.Phone),
		nullableLeadText(params.Email),
		params.SourceChannel,
		params.PipelineStatus,
		nullableLeadText(params.CampaignID),
		nullableLeadText(params.AssignedTo),
		nullableLeadText(params.Notes),
		nullableLeadText(params.CompanyName),
		params.EstimatedValue,
	)
	if err != nil {
		return model.Lead{}, err
	}
	if tag.RowsAffected() == 0 {
		return model.Lead{}, ErrLeadNotFound
	}

	return r.GetLeadByID(ctx, leadID)
}

func (r *LeadsRepository) DeleteLead(ctx context.Context, leadID string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM leads WHERE id = $1::uuid`, leadID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrLeadNotFound
	}
	return nil
}

func (r *LeadsRepository) ListPipeline(ctx context.Context) ([]model.LeadPipelineColumn, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			leads.id::text,
			leads.name,
			leads.phone,
			leads.email,
			leads.source_channel,
			leads.pipeline_status,
			leads.campaign_id::text,
			campaigns.name,
			leads.assigned_to::text,
			employees.full_name,
			employees.avatar_url,
			leads.notes,
			leads.company_name,
			leads.estimated_value,
			leads.created_by::text,
			users.full_name,
			leads.created_at,
			leads.updated_at
		FROM leads
		LEFT JOIN campaigns ON campaigns.id = leads.campaign_id
		LEFT JOIN employees ON employees.id = leads.assigned_to
		LEFT JOIN users ON users.id = leads.created_by
		ORDER BY leads.updated_at DESC, leads.created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := make([]model.LeadPipelineColumn, 0, len(leadStatuses))
	columnMap := make(map[string]*model.LeadPipelineColumn, len(leadStatuses))
	for _, status := range leadStatuses {
		column := model.LeadPipelineColumn{
			Status: status.Key,
			Label:  status.Label,
			Leads:  []model.Lead{},
		}
		columns = append(columns, column)
		columnMap[status.Key] = &columns[len(columns)-1]
	}

	for rows.Next() {
		var item model.Lead
		if err := scanLead(rows, &item); err != nil {
			return nil, err
		}
		column, ok := columnMap[item.PipelineStatus]
		if !ok {
			continue
		}
		column.Leads = append(column.Leads, item)
	}

	return columns, rows.Err()
}

func (r *LeadsRepository) MoveLeadStatus(ctx context.Context, leadID string, status string, actorID string) (model.Lead, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return model.Lead{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	var oldStatus string
	if err = tx.QueryRow(ctx, `SELECT pipeline_status FROM leads WHERE id = $1::uuid`, leadID).Scan(&oldStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Lead{}, ErrLeadNotFound
		}
		return model.Lead{}, err
	}

	if _, err = tx.Exec(ctx, `UPDATE leads SET pipeline_status = $2, updated_at = NOW() WHERE id = $1::uuid`, leadID, status); err != nil {
		return model.Lead{}, err
	}

	oldStatusCopy := oldStatus
	newStatusCopy := status
	if err = r.createActivityWithinTx(ctx, tx, CreateLeadActivityParams{
		LeadID:       leadID,
		ActivityType: "status_change",
		Description:  fmt.Sprintf("Lead moved from %s to %s", labelForLeadStatus(oldStatus), labelForLeadStatus(status)),
		OldStatus:    &oldStatusCopy,
		NewStatus:    &newStatusCopy,
		CreatedBy:    actorID,
	}); err != nil {
		return model.Lead{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return model.Lead{}, err
	}

	return r.GetLeadByID(ctx, leadID)
}

func (r *LeadsRepository) ListActivities(ctx context.Context, leadID string) ([]model.LeadActivity, error) {
	if _, err := r.GetLeadByID(ctx, leadID); err != nil {
		return nil, err
	}

	rows, err := r.db.Query(ctx, `
		SELECT
			lead_activities.id::text,
			lead_activities.lead_id::text,
			lead_activities.activity_type,
			lead_activities.description,
			lead_activities.old_status,
			lead_activities.new_status,
			lead_activities.created_by::text,
			users.full_name,
			lead_activities.created_at
		FROM lead_activities
		LEFT JOIN users ON users.id = lead_activities.created_by
		WHERE lead_activities.lead_id = $1::uuid
		ORDER BY lead_activities.created_at DESC
	`, leadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.LeadActivity, 0)
	for rows.Next() {
		var item model.LeadActivity
		if err := rows.Scan(
			&item.ID,
			&item.LeadID,
			&item.ActivityType,
			&item.Description,
			&item.OldStatus,
			&item.NewStatus,
			&item.CreatedBy,
			&item.CreatedByName,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

func (r *LeadsRepository) CreateActivity(ctx context.Context, params CreateLeadActivityParams) (model.LeadActivity, error) {
	if _, err := r.GetLeadByID(ctx, params.LeadID); err != nil {
		return model.LeadActivity{}, err
	}

	var item model.LeadActivity
	err := r.db.QueryRow(ctx, `
		INSERT INTO lead_activities (lead_id, activity_type, description, old_status, new_status, created_by)
		VALUES ($1::uuid, $2, $3, $4, $5, $6::uuid)
		RETURNING id::text, lead_id::text, activity_type, description, old_status, new_status, created_by::text, created_at
	`,
		params.LeadID,
		params.ActivityType,
		params.Description,
		nullableLeadText(params.OldStatus),
		nullableLeadText(params.NewStatus),
		params.CreatedBy,
	).Scan(
		&item.ID,
		&item.LeadID,
		&item.ActivityType,
		&item.Description,
		&item.OldStatus,
		&item.NewStatus,
		&item.CreatedBy,
		&item.CreatedAt,
	)
	if err != nil {
		return model.LeadActivity{}, err
	}

	if err := r.db.QueryRow(ctx, `SELECT full_name FROM users WHERE id = $1::uuid`, item.CreatedBy).Scan(&item.CreatedByName); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return model.LeadActivity{}, err
	}

	return item, nil
}

func (r *LeadsRepository) Summary(ctx context.Context) (model.LeadSummary, error) {
	rows, err := r.db.Query(ctx, `
		SELECT pipeline_status, COUNT(*)::bigint, COALESCE(SUM(estimated_value), 0)::bigint
		FROM leads
		GROUP BY pipeline_status
	`)
	if err != nil {
		return model.LeadSummary{}, err
	}
	defer rows.Close()

	byStatusMap := make(map[string]model.LeadSummaryRow, len(leadStatuses))
	for _, status := range leadStatuses {
		byStatusMap[status.Key] = model.LeadSummaryRow{
			Status: status.Key,
			Label:  status.Label,
		}
	}

	var totalLeads int64
	var wonLeads int64

	for rows.Next() {
		var status string
		var count int64
		var estimatedValue int64
		if err := rows.Scan(&status, &count, &estimatedValue); err != nil {
			return model.LeadSummary{}, err
		}
		row := byStatusMap[status]
		row.LeadCount = count
		row.EstimatedValue = estimatedValue
		byStatusMap[status] = row
		totalLeads += count
		if status == "won" {
			wonLeads += count
		}
	}

	items := make([]model.LeadSummaryRow, 0, len(leadStatuses))
	for _, status := range leadStatuses {
		items = append(items, byStatusMap[status.Key])
	}

	conversionRate := 0.0
	if totalLeads > 0 {
		conversionRate = float64(wonLeads) / float64(totalLeads) * 100
	}

	return model.LeadSummary{
		TotalLeads:     totalLeads,
		WonLeads:       wonLeads,
		ConversionRate: conversionRate,
		ByStatus:       items,
	}, nil
}

func (r *LeadsRepository) createActivityWithinTx(ctx context.Context, tx pgx.Tx, params CreateLeadActivityParams) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO lead_activities (lead_id, activity_type, description, old_status, new_status, created_by)
		VALUES ($1::uuid, $2, $3, $4, $5, $6::uuid)
	`,
		params.LeadID,
		params.ActivityType,
		params.Description,
		nullableLeadText(params.OldStatus),
		nullableLeadText(params.NewStatus),
		params.CreatedBy,
	)
	return err
}

func (r *LeadsRepository) ensureEmployeeExists(ctx context.Context, employeeID *string) error {
	if employeeID == nil || strings.TrimSpace(*employeeID) == "" {
		return nil
	}

	var exists bool
	if err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM employees WHERE id = $1::uuid)`, strings.TrimSpace(*employeeID)).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrLeadAssignedUserMissing
	}

	return nil
}

func (r *LeadsRepository) ensureCampaignExists(ctx context.Context, campaignID *string) error {
	if campaignID == nil || strings.TrimSpace(*campaignID) == "" {
		return nil
	}

	var exists bool
	if err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM campaigns WHERE id = $1::uuid)`, strings.TrimSpace(*campaignID)).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrLeadCampaignMissing
	}

	return nil
}

func scanLead(rows pgx.Rows, item *model.Lead) error {
	return rows.Scan(
		&item.ID,
		&item.Name,
		&item.Phone,
		&item.Email,
		&item.SourceChannel,
		&item.PipelineStatus,
		&item.CampaignID,
		&item.CampaignName,
		&item.AssignedTo,
		&item.AssignedToName,
		&item.AssignedToAvatar,
		&item.Notes,
		&item.CompanyName,
		&item.EstimatedValue,
		&item.CreatedBy,
		&item.CreatedByName,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
}

func scanLeadRow(row pgx.Row, item *model.Lead) error {
	return row.Scan(
		&item.ID,
		&item.Name,
		&item.Phone,
		&item.Email,
		&item.SourceChannel,
		&item.PipelineStatus,
		&item.CampaignID,
		&item.CampaignName,
		&item.AssignedTo,
		&item.AssignedToName,
		&item.AssignedToAvatar,
		&item.Notes,
		&item.CompanyName,
		&item.EstimatedValue,
		&item.CreatedBy,
		&item.CreatedByName,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
}

func nullableLeadText(value *string) interface{} {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

type leadStatusOption struct {
	Key   string
	Label string
}

var leadStatuses = []leadStatusOption{
	{Key: "new", Label: "New"},
	{Key: "contacted", Label: "Contacted"},
	{Key: "qualified", Label: "Qualified"},
	{Key: "proposal", Label: "Proposal"},
	{Key: "negotiation", Label: "Negotiation"},
	{Key: "won", Label: "Won"},
	{Key: "lost", Label: "Lost"},
}

func labelForLeadStatus(status string) string {
	for _, item := range leadStatuses {
		if item.Key == status {
			return item.Label
		}
	}
	return status
}
