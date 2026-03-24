package marketing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v5"

	"github.com/kana-consultant/kantor/backend/internal/model"
	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

var (
	ErrCampaignNotFound           = errors.New("campaign not found")
	ErrCampaignColumnNotFound     = errors.New("campaign column not found")
	ErrCampaignAttachmentNotFound = errors.New("campaign attachment not found")
	ErrCampaignPICNotFound        = errors.New("campaign pic employee not found")
	ErrCampaignColumnInUse        = errors.New("campaign column still has campaigns assigned")
)

type CampaignsRepository struct {
	db repository.DBTX
}

type ListCampaignsParams struct {
	Page     int
	PerPage  int
	Search   string
	Channel  string
	Status   string
	PIC      string
	DateFrom string
	DateTo   string
}

type UpsertCampaignParams struct {
	Name           string
	Description    *string
	Channel        string
	BudgetAmount   int64
	BudgetCurrency string
	PICEmployeeID  *string
	StartDate      time.Time
	EndDate        time.Time
	BriefText      *string
	Status         string
	ActorID        string
}

type CreateCampaignColumnParams struct {
	Name     string
	Color    *string
	Position *int
}

type UpdateCampaignColumnParams struct {
	Name  string
	Color *string
}

type CreateCampaignAttachmentParams struct {
	CampaignID string
	FileName   string
	FilePath   string
	FileType   string
	FileSize   int64
	UploadedBy string
}

type queryExecutor interface {
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
}

func NewCampaignsRepository(db repository.DBTX) *CampaignsRepository {
	return &CampaignsRepository{db: db}
}

func (r *CampaignsRepository) CreateCampaign(ctx context.Context, params UpsertCampaignParams) (model.Campaign, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	if err := r.ensureEmployeeExists(ctx, params.PICEmployeeID); err != nil {
		return model.Campaign{}, err
	}

	tx, err := repository.DB(ctx, r.db).Begin(ctx)
	if err != nil {
		return model.Campaign{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	var campaignID string
	err = tx.QueryRow(
		ctx,
		`
			INSERT INTO campaigns (
				name, description, channel, budget_amount, budget_currency, pic_employee_id, start_date, end_date, brief_text, status, created_by
			)
			VALUES ($1, NULLIF($2, ''), $3, $4, $5, NULLIF($6, '')::uuid, $7::date, $8::date, NULLIF($9, ''), $10, $11::uuid)
			RETURNING id::text
		`,
		params.Name,
		nullableString(params.Description),
		params.Channel,
		params.BudgetAmount,
		params.BudgetCurrency,
		nullableUUID(params.PICEmployeeID),
		params.StartDate,
		params.EndDate,
		nullableString(params.BriefText),
		params.Status,
		params.ActorID,
	).Scan(&campaignID)
	if err != nil {
		return model.Campaign{}, err
	}

	column, err := r.findColumnForStatus(ctx, tx, params.Status)
	if err != nil {
		return model.Campaign{}, err
	}

	position, err := r.resolveCampaignInsertPosition(ctx, tx, column.ID, nil)
	if err != nil {
		return model.Campaign{}, err
	}

	_, err = tx.Exec(
		ctx,
		`
			INSERT INTO campaign_column_assignments (campaign_id, column_id, position, moved_at, moved_by)
			VALUES ($1::uuid, $2::uuid, $3, NOW(), $4::uuid)
		`,
		campaignID,
		column.ID,
		position,
		params.ActorID,
	)
	if err != nil {
		return model.Campaign{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return model.Campaign{}, err
	}

	return r.GetCampaignByID(ctx, campaignID)
}

func (r *CampaignsRepository) ListCampaigns(ctx context.Context, params ListCampaignsParams) ([]model.Campaign, int64, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	filters := []string{"1=1"}
	args := make([]interface{}, 0)
	index := 1

	if search := strings.TrimSpace(params.Search); search != "" {
		filters = append(filters, fmt.Sprintf("campaigns.name ILIKE $%d", index))
		args = append(args, "%"+search+"%")
		index++
	}

	if channel := strings.TrimSpace(params.Channel); channel != "" {
		filters = append(filters, fmt.Sprintf("campaigns.channel = $%d", index))
		args = append(args, channel)
		index++
	}

	if status := strings.TrimSpace(params.Status); status != "" {
		filters = append(filters, fmt.Sprintf("campaigns.status = $%d", index))
		args = append(args, status)
		index++
	}

	if pic := strings.TrimSpace(params.PIC); pic != "" {
		filters = append(filters, fmt.Sprintf("campaigns.pic_employee_id = $%d::uuid", index))
		args = append(args, pic)
		index++
	}

	if dateFrom := strings.TrimSpace(params.DateFrom); dateFrom != "" {
		filters = append(filters, fmt.Sprintf("campaigns.end_date >= $%d::date", index))
		args = append(args, dateFrom)
		index++
	}

	if dateTo := strings.TrimSpace(params.DateTo); dateTo != "" {
		filters = append(filters, fmt.Sprintf("campaigns.start_date <= $%d::date", index))
		args = append(args, dateTo)
		index++
	}

	whereClause := strings.Join(filters, " AND ")

	var total int64
	if err := repository.DB(ctx, r.db).QueryRow(ctx, `SELECT COUNT(*) FROM campaigns WHERE `+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.PerPage
	query := fmt.Sprintf(`
		SELECT
			campaigns.id::text,
			campaigns.name,
			campaigns.description,
			campaigns.channel,
			campaigns.budget_amount,
			campaigns.budget_currency,
			campaigns.pic_employee_id::text,
			employees.full_name,
			employees.avatar_url,
			campaigns.start_date,
			campaigns.end_date,
			campaigns.brief_text,
			campaigns.status,
			campaigns.created_by::text,
			campaigns.created_at,
			campaigns.updated_at,
			campaign_column_assignments.column_id::text,
			campaign_columns.name,
			campaign_columns.color,
			campaign_column_assignments.position,
			COUNT(campaign_attachments.id)::int AS attachment_count
		FROM campaigns
		LEFT JOIN employees ON employees.id = campaigns.pic_employee_id
		LEFT JOIN campaign_column_assignments ON campaign_column_assignments.campaign_id = campaigns.id
		LEFT JOIN campaign_columns ON campaign_columns.id = campaign_column_assignments.column_id
		LEFT JOIN campaign_attachments ON campaign_attachments.campaign_id = campaigns.id
		WHERE %s
		GROUP BY campaigns.id, employees.full_name, employees.avatar_url, campaign_column_assignments.column_id, campaign_columns.name, campaign_columns.color, campaign_column_assignments.position, campaign_columns.position
		ORDER BY COALESCE(campaign_columns.position, 9999) ASC, COALESCE(campaign_column_assignments.position, 9999) ASC, campaigns.updated_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, index, index+1)
	args = append(args, params.PerPage, offset)

	rows, err := repository.DB(ctx, r.db).Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]model.Campaign, 0)
	for rows.Next() {
		var item model.Campaign
		if err := scanCampaign(rows, &item); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}

	return items, total, rows.Err()
}

func (r *CampaignsRepository) GetCampaignByID(ctx context.Context, campaignID string) (model.Campaign, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	row := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT
			campaigns.id::text,
			campaigns.name,
			campaigns.description,
			campaigns.channel,
			campaigns.budget_amount,
			campaigns.budget_currency,
			campaigns.pic_employee_id::text,
			employees.full_name,
			employees.avatar_url,
			campaigns.start_date,
			campaigns.end_date,
			campaigns.brief_text,
			campaigns.status,
			campaigns.created_by::text,
			campaigns.created_at,
			campaigns.updated_at,
			campaign_column_assignments.column_id::text,
			campaign_columns.name,
			campaign_columns.color,
			campaign_column_assignments.position,
			COUNT(campaign_attachments.id)::int AS attachment_count
		FROM campaigns
		LEFT JOIN employees ON employees.id = campaigns.pic_employee_id
		LEFT JOIN campaign_column_assignments ON campaign_column_assignments.campaign_id = campaigns.id
		LEFT JOIN campaign_columns ON campaign_columns.id = campaign_column_assignments.column_id
		LEFT JOIN campaign_attachments ON campaign_attachments.campaign_id = campaigns.id
		WHERE campaigns.id = $1::uuid
		GROUP BY campaigns.id, employees.full_name, employees.avatar_url, campaign_column_assignments.column_id, campaign_columns.name, campaign_columns.color, campaign_column_assignments.position
	`, campaignID)

	var item model.Campaign
	if err := scanCampaignRow(row, &item); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Campaign{}, ErrCampaignNotFound
		}
		return model.Campaign{}, err
	}

	return item, nil
}

func (r *CampaignsRepository) UpdateCampaign(ctx context.Context, campaignID string, params UpsertCampaignParams) (model.Campaign, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	if err := r.ensureEmployeeExists(ctx, params.PICEmployeeID); err != nil {
		return model.Campaign{}, err
	}

	tx, err := repository.DB(ctx, r.db).Begin(ctx)
	if err != nil {
		return model.Campaign{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	tag, err := tx.Exec(
		ctx,
		`
			UPDATE campaigns
			SET
				name = $2,
				description = NULLIF($3, ''),
				channel = $4,
				budget_amount = $5,
				budget_currency = $6,
				pic_employee_id = NULLIF($7, '')::uuid,
				start_date = $8::date,
				end_date = $9::date,
				brief_text = NULLIF($10, ''),
				status = $11,
				updated_at = NOW()
			WHERE id = $1::uuid
		`,
		campaignID,
		params.Name,
		nullableString(params.Description),
		params.Channel,
		params.BudgetAmount,
		params.BudgetCurrency,
		nullableUUID(params.PICEmployeeID),
		params.StartDate,
		params.EndDate,
		nullableString(params.BriefText),
		params.Status,
	)
	if err != nil {
		return model.Campaign{}, err
	}
	if tag.RowsAffected() == 0 {
		return model.Campaign{}, ErrCampaignNotFound
	}

	column, err := r.findColumnForStatus(ctx, tx, params.Status)
	if err != nil {
		return model.Campaign{}, err
	}
	if err = r.moveCampaignWithinTx(ctx, tx, campaignID, column.ID, nil, params.ActorID, params.Status); err != nil {
		return model.Campaign{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return model.Campaign{}, err
	}

	return r.GetCampaignByID(ctx, campaignID)
}

func (r *CampaignsRepository) DeleteCampaign(ctx context.Context, campaignID string) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tag, err := repository.DB(ctx, r.db).Exec(ctx, `DELETE FROM campaigns WHERE id = $1::uuid`, campaignID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrCampaignNotFound
	}
	return nil
}

func (r *CampaignsRepository) ListKanban(ctx context.Context) ([]model.CampaignColumn, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	columns, err := r.ListColumns(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT
			campaigns.id::text,
			campaigns.name,
			campaigns.description,
			campaigns.channel,
			campaigns.budget_amount,
			campaigns.budget_currency,
			campaigns.pic_employee_id::text,
			employees.full_name,
			employees.avatar_url,
			campaigns.start_date,
			campaigns.end_date,
			campaigns.brief_text,
			campaigns.status,
			campaigns.created_by::text,
			campaigns.created_at,
			campaigns.updated_at,
			campaign_column_assignments.column_id::text,
			campaign_columns.name,
			campaign_columns.color,
			campaign_column_assignments.position,
			COUNT(campaign_attachments.id)::int AS attachment_count
		FROM campaign_column_assignments
		INNER JOIN campaigns ON campaigns.id = campaign_column_assignments.campaign_id
		INNER JOIN campaign_columns ON campaign_columns.id = campaign_column_assignments.column_id
		LEFT JOIN employees ON employees.id = campaigns.pic_employee_id
		LEFT JOIN campaign_attachments ON campaign_attachments.campaign_id = campaigns.id
		GROUP BY campaigns.id, employees.full_name, employees.avatar_url, campaign_column_assignments.column_id, campaign_columns.name, campaign_columns.color, campaign_column_assignments.position, campaign_columns.position
		ORDER BY campaign_columns.position ASC, campaign_column_assignments.position ASC, campaigns.created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columnMap := make(map[string]*model.CampaignColumn, len(columns))
	for index := range columns {
		columns[index].Campaigns = []model.Campaign{}
		columnMap[columns[index].ID] = &columns[index]
	}

	for rows.Next() {
		var item model.Campaign
		if err := scanCampaign(rows, &item); err != nil {
			return nil, err
		}
		if item.ColumnID == nil {
			continue
		}
		column, ok := columnMap[*item.ColumnID]
		if !ok {
			continue
		}
		column.Campaigns = append(column.Campaigns, item)
		column.CampaignsNo = len(column.Campaigns)
	}

	return columns, rows.Err()
}

func (r *CampaignsRepository) MoveCampaign(ctx context.Context, campaignID string, columnID string, position int, movedBy string) (model.Campaign, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tx, err := repository.DB(ctx, r.db).Begin(ctx)
	if err != nil {
		return model.Campaign{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	status, err := r.statusForColumnID(ctx, tx, columnID)
	if err != nil {
		return model.Campaign{}, err
	}

	if err = r.moveCampaignWithinTx(ctx, tx, campaignID, columnID, &position, movedBy, status); err != nil {
		return model.Campaign{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return model.Campaign{}, err
	}

	return r.GetCampaignByID(ctx, campaignID)
}

func (r *CampaignsRepository) ListColumns(ctx context.Context) ([]model.CampaignColumn, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT id::text, name, position, color, created_at
		FROM campaign_columns
		ORDER BY position ASC, created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.CampaignColumn, 0)
	for rows.Next() {
		var item model.CampaignColumn
		if err := rows.Scan(&item.ID, &item.Name, &item.Position, &item.Color, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

func (r *CampaignsRepository) CreateColumn(ctx context.Context, params CreateCampaignColumnParams) (model.CampaignColumn, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tx, err := repository.DB(ctx, r.db).Begin(ctx)
	if err != nil {
		return model.CampaignColumn{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	position, err := r.resolveColumnInsertPosition(ctx, tx, params.Position)
	if err != nil {
		return model.CampaignColumn{}, err
	}

	if _, err = tx.Exec(ctx, `UPDATE campaign_columns SET position = position + 1 WHERE position >= $1`, position); err != nil {
		return model.CampaignColumn{}, err
	}

	var item model.CampaignColumn
	err = tx.QueryRow(
		ctx,
		`
			INSERT INTO campaign_columns (name, position, color)
			VALUES ($1, $2, NULLIF($3, ''))
			RETURNING id::text, name, position, color, created_at
		`,
		params.Name,
		position,
		nullableString(params.Color),
	).Scan(&item.ID, &item.Name, &item.Position, &item.Color, &item.CreatedAt)
	if err != nil {
		return model.CampaignColumn{}, mapCampaignDBError(err)
	}

	if err = tx.Commit(ctx); err != nil {
		return model.CampaignColumn{}, err
	}

	return item, nil
}

func (r *CampaignsRepository) UpdateColumn(ctx context.Context, columnID string, params UpdateCampaignColumnParams) (model.CampaignColumn, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var item model.CampaignColumn
	err := repository.DB(ctx, r.db).QueryRow(
		ctx,
		`
			UPDATE campaign_columns
			SET name = $2, color = NULLIF($3, '')
			WHERE id = $1::uuid
			RETURNING id::text, name, position, color, created_at
		`,
		columnID,
		params.Name,
		nullableString(params.Color),
	).Scan(&item.ID, &item.Name, &item.Position, &item.Color, &item.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.CampaignColumn{}, ErrCampaignColumnNotFound
		}
		return model.CampaignColumn{}, mapCampaignDBError(err)
	}

	return item, nil
}

func (r *CampaignsRepository) DeleteColumn(ctx context.Context, columnID string) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var campaignCount int
	if err := repository.DB(ctx, r.db).QueryRow(ctx, `SELECT COUNT(*) FROM campaign_column_assignments WHERE column_id = $1::uuid`, columnID).Scan(&campaignCount); err != nil {
		return err
	}
	if campaignCount > 0 {
		return ErrCampaignColumnInUse
	}

	tx, err := repository.DB(ctx, r.db).Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	var position int
	err = tx.QueryRow(ctx, `DELETE FROM campaign_columns WHERE id = $1::uuid RETURNING position`, columnID).Scan(&position)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCampaignColumnNotFound
		}
		return err
	}

	if _, err = tx.Exec(ctx, `UPDATE campaign_columns SET position = position - 1 WHERE position > $1`, position); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *CampaignsRepository) ReorderColumns(ctx context.Context, columnIDs []string) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tx, err := repository.DB(ctx, r.db).Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	var count int
	if err = tx.QueryRow(ctx, `SELECT COUNT(*) FROM campaign_columns`).Scan(&count); err != nil {
		return err
	}
	if count != len(columnIDs) {
		return fmt.Errorf("column reorder payload must contain every campaign column")
	}

	for index, columnID := range columnIDs {
		tag, execErr := tx.Exec(ctx, `UPDATE campaign_columns SET position = $2 WHERE id = $1::uuid`, columnID, index+1)
		if execErr != nil {
			return execErr
		}
		if tag.RowsAffected() == 0 {
			return ErrCampaignColumnNotFound
		}
	}

	return tx.Commit(ctx)
}

func (r *CampaignsRepository) CreateAttachment(ctx context.Context, params CreateCampaignAttachmentParams) (model.CampaignAttachment, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var item model.CampaignAttachment
	err := repository.DB(ctx, r.db).QueryRow(
		ctx,
		`
			INSERT INTO campaign_attachments (campaign_id, file_name, file_path, file_type, file_size, uploaded_by)
			VALUES ($1::uuid, $2, $3, $4, $5, $6::uuid)
			RETURNING id::text, campaign_id::text, file_name, file_path, file_type, file_size, uploaded_by::text, created_at
		`,
		params.CampaignID,
		params.FileName,
		params.FilePath,
		params.FileType,
		params.FileSize,
		params.UploadedBy,
	).Scan(&item.ID, &item.CampaignID, &item.FileName, &item.FilePath, &item.FileType, &item.FileSize, &item.UploadedBy, &item.CreatedAt)
	if err != nil {
		if isForeignKeyError(err) {
			return model.CampaignAttachment{}, ErrCampaignNotFound
		}
		return model.CampaignAttachment{}, err
	}
	return item, nil
}

func (r *CampaignsRepository) ListAttachments(ctx context.Context, campaignID string) ([]model.CampaignAttachment, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	if _, err := r.GetCampaignByID(ctx, campaignID); err != nil {
		return nil, err
	}

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT id::text, campaign_id::text, file_name, file_path, file_type, file_size, uploaded_by::text, created_at
		FROM campaign_attachments
		WHERE campaign_id = $1::uuid
		ORDER BY created_at DESC
	`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.CampaignAttachment, 0)
	for rows.Next() {
		var item model.CampaignAttachment
		if err := rows.Scan(&item.ID, &item.CampaignID, &item.FileName, &item.FilePath, &item.FileType, &item.FileSize, &item.UploadedBy, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

func (r *CampaignsRepository) DeleteAttachment(ctx context.Context, campaignID string, attachmentID string) (model.CampaignAttachment, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var item model.CampaignAttachment
	err := repository.DB(ctx, r.db).QueryRow(
		ctx,
		`
			DELETE FROM campaign_attachments
			WHERE campaign_id = $1::uuid AND id = $2::uuid
			RETURNING id::text, campaign_id::text, file_name, file_path, file_type, file_size, uploaded_by::text, created_at
		`,
		campaignID,
		attachmentID,
	).Scan(&item.ID, &item.CampaignID, &item.FileName, &item.FilePath, &item.FileType, &item.FileSize, &item.UploadedBy, &item.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.CampaignAttachment{}, ErrCampaignAttachmentNotFound
		}
		return model.CampaignAttachment{}, err
	}
	return item, nil
}

func (r *CampaignsRepository) ListActivities(ctx context.Context, campaignID string) ([]model.CampaignActivity, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	if _, err := r.GetCampaignByID(ctx, campaignID); err != nil {
		return nil, err
	}

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT
			audit_logs.id::text,
			audit_logs.resource_id,
			audit_logs.action,
			CASE
				WHEN audit_logs.action = 'campaign_moved' THEN CONCAT('Moved to ', COALESCE(audit_logs.new_value->>'column_name', 'another stage'))
				WHEN audit_logs.action = 'attachment_uploaded' THEN CONCAT('Uploaded ', COALESCE(audit_logs.new_value->>'file_name', 'an attachment'))
				ELSE REPLACE(INITCAP(REPLACE(audit_logs.action, '_', ' ')), '  ', ' ')
			END AS description,
			audit_logs.user_id::text,
			users.full_name,
			audit_logs.created_at
		FROM audit_logs
		LEFT JOIN users ON users.id = audit_logs.user_id
		WHERE audit_logs.module = 'marketing' AND audit_logs.resource = 'campaign' AND audit_logs.resource_id = $1
		ORDER BY audit_logs.created_at DESC
	`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.CampaignActivity, 0)
	for rows.Next() {
		var item model.CampaignActivity
		if err := rows.Scan(
			&item.ID,
			&item.CampaignID,
			&item.Action,
			&item.Description,
			&item.ActorID,
			&item.ActorName,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

func (r *CampaignsRepository) LogActivity(ctx context.Context, campaignID string, actorID string, action string, payload map[string]any) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	newValue, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = repository.DB(ctx, r.db).Exec(ctx, `
		INSERT INTO audit_logs (user_id, action, module, resource, resource_id, new_value, created_at)
		VALUES ($1::uuid, $2, 'marketing', 'campaign', $3, $4::jsonb, NOW())
	`, actorID, action, campaignID, newValue)
	return err
}

func (r *CampaignsRepository) FindAttachmentPath(ctx context.Context, campaignID string, filename string) (string, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	if _, err := r.GetCampaignByID(ctx, campaignID); err != nil {
		return "", err
	}

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT file_path
		FROM campaign_attachments
		WHERE campaign_id = $1::uuid
	`, campaignID)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	target := filepath.Base(strings.TrimSpace(filename))
	for rows.Next() {
		var filePath string
		if err := rows.Scan(&filePath); err != nil {
			return "", err
		}
		if filepath.Base(filepath.FromSlash(filePath)) == target {
			return filePath, nil
		}
	}

	if err := rows.Err(); err != nil {
		return "", err
	}

	return "", ErrCampaignAttachmentNotFound
}

func (r *CampaignsRepository) moveCampaignWithinTx(ctx context.Context, tx pgx.Tx, campaignID string, destinationColumnID string, requestedPosition *int, movedBy string, status string) error {
	if err := r.ensureCampaignExists(ctx, tx, campaignID); err != nil {
		return err
	}
	if err := r.ensureColumnExists(ctx, tx, destinationColumnID); err != nil {
		return err
	}

	var currentColumnID string
	var currentPosition int
	currentAssigned := true
	err := tx.QueryRow(ctx, `SELECT column_id::text, position FROM campaign_column_assignments WHERE campaign_id = $1::uuid`, campaignID).Scan(&currentColumnID, &currentPosition)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			currentAssigned = false
		} else {
			return err
		}
	}

	position, err := r.resolveCampaignInsertPosition(ctx, tx, destinationColumnID, requestedPosition)
	if err != nil {
		return err
	}

	if currentAssigned {
		if currentColumnID == destinationColumnID {
			maxPosition, err := r.maxCampaignPosition(ctx, tx, destinationColumnID)
			if err != nil {
				return err
			}
			if position > maxPosition {
				position = maxPosition
			}
			if position != currentPosition {
				if position < currentPosition {
					_, err = tx.Exec(ctx, `UPDATE campaign_column_assignments SET position = position + 1 WHERE column_id = $1::uuid AND position >= $2 AND position < $3`, destinationColumnID, position, currentPosition)
				} else {
					_, err = tx.Exec(ctx, `UPDATE campaign_column_assignments SET position = position - 1 WHERE column_id = $1::uuid AND position > $2 AND position <= $3`, destinationColumnID, currentPosition, position)
				}
				if err != nil {
					return err
				}
			}
		} else {
			if _, err = tx.Exec(ctx, `UPDATE campaign_column_assignments SET position = position - 1 WHERE column_id = $1::uuid AND position > $2`, currentColumnID, currentPosition); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `UPDATE campaign_column_assignments SET position = position + 1 WHERE column_id = $1::uuid AND position >= $2`, destinationColumnID, position); err != nil {
				return err
			}
		}
	} else {
		if _, err = tx.Exec(ctx, `UPDATE campaign_column_assignments SET position = position + 1 WHERE column_id = $1::uuid AND position >= $2`, destinationColumnID, position); err != nil {
			return err
		}
	}

	_, err = tx.Exec(
		ctx,
		`
			INSERT INTO campaign_column_assignments (campaign_id, column_id, position, moved_at, moved_by)
			VALUES ($1::uuid, $2::uuid, $3, NOW(), $4::uuid)
			ON CONFLICT (campaign_id)
			DO UPDATE SET column_id = EXCLUDED.column_id, position = EXCLUDED.position, moved_at = NOW(), moved_by = EXCLUDED.moved_by
		`,
		campaignID,
		destinationColumnID,
		position,
		movedBy,
	)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `UPDATE campaigns SET status = $2, updated_at = NOW() WHERE id = $1::uuid`, campaignID, status)
	return err
}

func (r *CampaignsRepository) ensureCampaignExists(ctx context.Context, tx queryExecutor, campaignID string) error {
	var exists bool
	err := tx.QueryRow(ctx, `SELECT TRUE FROM campaigns WHERE id = $1::uuid`, campaignID).Scan(&exists)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCampaignNotFound
		}
		return err
	}
	return nil
}

func (r *CampaignsRepository) ensureColumnExists(ctx context.Context, tx queryExecutor, columnID string) error {
	var exists bool
	err := tx.QueryRow(ctx, `SELECT TRUE FROM campaign_columns WHERE id = $1::uuid`, columnID).Scan(&exists)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCampaignColumnNotFound
		}
		return err
	}
	return nil
}

func (r *CampaignsRepository) ensureEmployeeExists(ctx context.Context, employeeID *string) error {
	if employeeID == nil || strings.TrimSpace(*employeeID) == "" {
		return nil
	}
	var exists bool
	if err := repository.DB(ctx, r.db).QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM employees WHERE id = $1::uuid)`, strings.TrimSpace(*employeeID)).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrCampaignPICNotFound
	}
	return nil
}

func (r *CampaignsRepository) findColumnForStatus(ctx context.Context, tx queryExecutor, status string) (model.CampaignColumn, error) {
	rows, err := tx.Query(ctx, `SELECT id::text, name, position, color, created_at FROM campaign_columns ORDER BY position ASC, created_at ASC`)
	if err != nil {
		return model.CampaignColumn{}, err
	}
	defer rows.Close()

	target := strings.TrimSpace(status)
	for rows.Next() {
		var item model.CampaignColumn
		if err := rows.Scan(&item.ID, &item.Name, &item.Position, &item.Color, &item.CreatedAt); err != nil {
			return model.CampaignColumn{}, err
		}
		if canonicalCampaignState(item.Name) == target {
			return item, nil
		}
	}

	if err := rows.Err(); err != nil {
		return model.CampaignColumn{}, err
	}

	return model.CampaignColumn{}, ErrCampaignColumnNotFound
}

func (r *CampaignsRepository) statusForColumnID(ctx context.Context, tx queryExecutor, columnID string) (string, error) {
	var name string
	err := tx.QueryRow(ctx, `SELECT name FROM campaign_columns WHERE id = $1::uuid`, columnID).Scan(&name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrCampaignColumnNotFound
		}
		return "", err
	}

	status := canonicalCampaignState(name)
	if status == "" {
		return "planning", nil
	}
	return status, nil
}

func (r *CampaignsRepository) maxCampaignPosition(ctx context.Context, tx queryExecutor, columnID string) (int, error) {
	var maxPosition int
	err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(position), 0) FROM campaign_column_assignments WHERE column_id = $1::uuid`, columnID).Scan(&maxPosition)
	return maxPosition, err
}

func (r *CampaignsRepository) resolveCampaignInsertPosition(ctx context.Context, tx queryExecutor, columnID string, requested *int) (int, error) {
	maxPosition, err := r.maxCampaignPosition(ctx, tx, columnID)
	if err != nil {
		return 0, err
	}
	if requested == nil || *requested > maxPosition+1 {
		return maxPosition + 1, nil
	}
	if *requested < 1 {
		return 1, nil
	}
	return *requested, nil
}

func (r *CampaignsRepository) resolveColumnInsertPosition(ctx context.Context, tx queryExecutor, requested *int) (int, error) {
	var maxPosition int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(position), 0) FROM campaign_columns`).Scan(&maxPosition); err != nil {
		return 0, err
	}
	if requested == nil || *requested > maxPosition+1 {
		return maxPosition + 1, nil
	}
	if *requested < 1 {
		return 1, nil
	}
	return *requested, nil
}

func scanCampaign(rows pgx.Rows, item *model.Campaign) error {
	return rows.Scan(
		&item.ID,
		&item.Name,
		&item.Description,
		&item.Channel,
		&item.BudgetAmount,
		&item.BudgetCurrency,
		&item.PICEmployeeID,
		&item.PICEmployeeName,
		&item.PICAvatarURL,
		&item.StartDate,
		&item.EndDate,
		&item.BriefText,
		&item.Status,
		&item.CreatedBy,
		&item.CreatedAt,
		&item.UpdatedAt,
		&item.ColumnID,
		&item.ColumnName,
		&item.ColumnColor,
		&item.ColumnPosition,
		&item.AttachmentCount,
	)
}

func scanCampaignRow(row pgx.Row, item *model.Campaign) error {
	return row.Scan(
		&item.ID,
		&item.Name,
		&item.Description,
		&item.Channel,
		&item.BudgetAmount,
		&item.BudgetCurrency,
		&item.PICEmployeeID,
		&item.PICEmployeeName,
		&item.PICAvatarURL,
		&item.StartDate,
		&item.EndDate,
		&item.BriefText,
		&item.Status,
		&item.CreatedBy,
		&item.CreatedAt,
		&item.UpdatedAt,
		&item.ColumnID,
		&item.ColumnName,
		&item.ColumnColor,
		&item.ColumnPosition,
		&item.AttachmentCount,
	)
}

var nonAlphaNumeric = regexp.MustCompile(`[^a-z0-9]+`)

func canonicalCampaignState(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = nonAlphaNumeric.ReplaceAllString(normalized, "_")
	normalized = strings.Trim(normalized, "_")
	switch normalized {
	case "ideation", "planning", "in_production", "live", "completed", "archived":
		return normalized
	default:
		return ""
	}
}

func nullableString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func nullableUUID(value *string) interface{} {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}
	return strings.TrimSpace(*value)
}

func mapCampaignDBError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}

	switch pgErr.ConstraintName {
	case "campaigns_pic_employee_id_fkey":
		return ErrCampaignPICNotFound
	}

	return err
}

func isForeignKeyError(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}
