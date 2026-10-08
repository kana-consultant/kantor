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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kana-consultant/kantor/backend/internal/model"
	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

var (
	ErrCampaignNotFound           = errors.New("campaign not found")
	ErrCampaignColumnNotFound     = errors.New("campaign column not found")
	ErrCampaignAttachmentNotFound = errors.New("campaign attachment not found")
	ErrCampaignPICNotFound        = errors.New("campaign pic employee not found")
	ErrCampaignColumnInUse        = errors.New("campaign column still has campaigns assigned")
	// ErrCampaignColumnProtected: the column carries a stage and cannot be deleted.
	ErrCampaignColumnProtected = errors.New("campaign stage column cannot be deleted")
	// ErrCampaignColumnNameTaken: another column of the tenant has this name.
	ErrCampaignColumnNameTaken = errors.New("campaign column name is already in use")
	// ErrCampaignColumnReorderInvalid: the reorder payload does not list every
	// column of the board exactly once.
	ErrCampaignColumnReorderInvalid = errors.New("column reorder payload must list every campaign column exactly once")
	// ErrCampaignDateRange and ErrCampaignChannelInvalid are the database
	// CHECKs behind the request validation, for writers that got past it.
	ErrCampaignDateRange      = errors.New("campaign end date is before its start date")
	ErrCampaignChannelInvalid = errors.New("campaign channel is not allowed")
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

// UpdateCampaignColumnParams: a nil Color keeps the stored colour, an empty
// one clears it.
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

func (r *CampaignsRepository) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return repository.DB(ctx, r.db).Begin(ctx)
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
	defer rollbackTx(tx)

	// Resolve the lane first: a stage without a column is repaired here, and
	// if it cannot be, nothing has been written yet.
	column, err := r.findStageColumn(ctx, tx, params.Status)
	if err != nil {
		return model.Campaign{}, err
	}
	if err = lockColumnsForPlacement(ctx, tx, column.ID); err != nil {
		return model.Campaign{}, err
	}

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
		return model.Campaign{}, mapCampaignDBError(err)
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
		ORDER BY COALESCE(campaign_columns.position, 9999) ASC, COALESCE(campaign_column_assignments.position, 9999) ASC, campaigns.updated_at DESC, campaigns.id ASC
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

// UpdateCampaign saves the campaign fields. The card is moved (to the end of
// the lane that carries the new stage) only when the stage changed or the
// campaign sits in no lane at all; an edit that keeps the stage leaves the
// card exactly where it is, custom lanes included. The second return value
// is the stage before the update.
func (r *CampaignsRepository) UpdateCampaign(ctx context.Context, campaignID string, params UpsertCampaignParams) (model.Campaign, string, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	if err := r.ensureEmployeeExists(ctx, params.PICEmployeeID); err != nil {
		return model.Campaign{}, "", err
	}

	tx, err := repository.DB(ctx, r.db).Begin(ctx)
	if err != nil {
		return model.Campaign{}, "", err
	}
	defer rollbackTx(tx)

	var (
		previousStatus string
		assigned       bool
	)
	err = tx.QueryRow(ctx, `SELECT status FROM campaigns WHERE id = $1::uuid FOR UPDATE`, campaignID).Scan(&previousStatus)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Campaign{}, "", ErrCampaignNotFound
		}
		return model.Campaign{}, "", err
	}
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM campaign_column_assignments WHERE campaign_id = $1::uuid)`, campaignID).Scan(&assigned); err != nil {
		return model.Campaign{}, "", err
	}

	_, err = tx.Exec(
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
		return model.Campaign{}, "", mapCampaignDBError(err)
	}

	if params.Status != previousStatus || !assigned {
		column, err := r.findStageColumn(ctx, tx, params.Status)
		if err != nil {
			return model.Campaign{}, "", err
		}
		status := params.Status
		if err = r.moveCampaignWithinTx(ctx, tx, campaignID, column.ID, nil, params.ActorID, &status); err != nil {
			return model.Campaign{}, "", err
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return model.Campaign{}, "", err
	}

	item, err := r.GetCampaignByID(ctx, campaignID)
	return item, previousStatus, err
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

// MoveCampaign puts the card into a lane. A lane that carries a stage sets
// the campaign's status to it; a custom lane leaves the status as it is.
func (r *CampaignsRepository) MoveCampaign(ctx context.Context, campaignID string, columnID string, position int, movedBy string) (model.Campaign, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tx, err := repository.DB(ctx, r.db).Begin(ctx)
	if err != nil {
		return model.Campaign{}, err
	}
	defer rollbackTx(tx)

	stage, err := r.stageForColumnID(ctx, tx, columnID)
	if err != nil {
		return model.Campaign{}, err
	}

	if err = r.moveCampaignWithinTx(ctx, tx, campaignID, columnID, &position, movedBy, stage); err != nil {
		return model.Campaign{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return model.Campaign{}, err
	}

	return r.GetCampaignByID(ctx, campaignID)
}

// ListColumns returns the board's lanes in order, each with the number of
// campaigns in it.
func (r *CampaignsRepository) ListColumns(ctx context.Context) ([]model.CampaignColumn, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT
			campaign_columns.id::text,
			campaign_columns.name,
			campaign_columns.position,
			campaign_columns.color,
			campaign_columns.stage,
			campaign_columns.created_at,
			(SELECT COUNT(*) FROM campaign_column_assignments WHERE campaign_column_assignments.column_id = campaign_columns.id)::int
		FROM campaign_columns
		ORDER BY campaign_columns.position ASC, campaign_columns.created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.CampaignColumn, 0)
	for rows.Next() {
		var item model.CampaignColumn
		if err := rows.Scan(&item.ID, &item.Name, &item.Position, &item.Color, &item.Stage, &item.CreatedAt, &item.CampaignsNo); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

// ListPICOptions returns the employees a campaign can be assigned to: the
// current staff (active or on probation) plus anyone who still is the person
// in charge of a campaign, so an existing choice can always be shown and
// filtered on. Only id, name, position and avatar leave the table.
func (r *CampaignsRepository) ListPICOptions(ctx context.Context) ([]model.CampaignPICOption, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT employees.id::text, employees.full_name, employees.position, employees.avatar_url
		FROM employees
		WHERE employees.employment_status IN ('active', 'probation')
			OR EXISTS (SELECT 1 FROM campaigns WHERE campaigns.pic_employee_id = employees.id)
		ORDER BY LOWER(employees.full_name) ASC, employees.id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.CampaignPICOption, 0)
	for rows.Next() {
		var item model.CampaignPICOption
		if err := rows.Scan(&item.ID, &item.FullName, &item.Position, &item.AvatarURL); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

// CreateColumn adds a custom lane (no stage) at the requested position, or
// after the last lane when none is given.
func (r *CampaignsRepository) CreateColumn(ctx context.Context, params CreateCampaignColumnParams) (model.CampaignColumn, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tx, err := repository.DB(ctx, r.db).Begin(ctx)
	if err != nil {
		return model.CampaignColumn{}, err
	}
	defer rollbackTx(tx)

	if err = lockCampaignColumnLayout(ctx, tx); err != nil {
		return model.CampaignColumn{}, err
	}
	maxPosition, err := maxColumnPosition(ctx, tx)
	if err != nil {
		return model.CampaignColumn{}, err
	}
	position := maxPosition + 1
	if params.Position != nil && *params.Position <= maxPosition {
		position = max(*params.Position, 1)
		// Make room: every lane from `position` on moves one step right.
		if err = shiftColumnPositions(ctx, tx, maxPosition, `position >= $2`, position, 1); err != nil {
			return model.CampaignColumn{}, err
		}
	}

	var item model.CampaignColumn
	err = tx.QueryRow(
		ctx,
		`
			INSERT INTO campaign_columns (name, position, color)
			VALUES ($1, $2, NULLIF($3, ''))
			RETURNING id::text, name, position, color, stage, created_at
		`,
		params.Name,
		position,
		nullableString(params.Color),
	).Scan(&item.ID, &item.Name, &item.Position, &item.Color, &item.Stage, &item.CreatedAt)
	if err != nil {
		return model.CampaignColumn{}, mapCampaignDBError(err)
	}

	if err = tx.Commit(ctx); err != nil {
		return model.CampaignColumn{}, err
	}

	return item, nil
}

// UpdateColumn renames and/or recolours a lane. It never changes the lane's
// stage: a renamed stage lane keeps standing for the same status.
func (r *CampaignsRepository) UpdateColumn(ctx context.Context, columnID string, params UpdateCampaignColumnParams) (model.CampaignColumn, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var color interface{}
	if params.Color != nil {
		color = strings.TrimSpace(*params.Color)
	}

	var item model.CampaignColumn
	err := repository.DB(ctx, r.db).QueryRow(
		ctx,
		`
			UPDATE campaign_columns
			SET name = $2, color = CASE WHEN $3::text IS NULL THEN color ELSE NULLIF($3::text, '') END
			WHERE id = $1::uuid
			RETURNING id::text, name, position, color, stage, created_at,
				(SELECT COUNT(*) FROM campaign_column_assignments WHERE campaign_column_assignments.column_id = campaign_columns.id)::int
		`,
		columnID,
		params.Name,
		color,
	).Scan(&item.ID, &item.Name, &item.Position, &item.Color, &item.Stage, &item.CreatedAt, &item.CampaignsNo)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.CampaignColumn{}, ErrCampaignColumnNotFound
		}
		return model.CampaignColumn{}, mapCampaignDBError(err)
	}

	return item, nil
}

// DeleteColumn removes an empty custom lane and closes the gap it leaves. A
// lane that carries a stage is never deleted (ErrCampaignColumnProtected).
func (r *CampaignsRepository) DeleteColumn(ctx context.Context, columnID string) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tx, err := repository.DB(ctx, r.db).Begin(ctx)
	if err != nil {
		return err
	}
	defer rollbackTx(tx)

	if err = lockCampaignColumnLayout(ctx, tx); err != nil {
		return err
	}

	var (
		stage    *string
		position int
	)
	err = tx.QueryRow(ctx, `SELECT stage, position FROM campaign_columns WHERE id = $1::uuid FOR UPDATE`, columnID).Scan(&stage, &position)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCampaignColumnNotFound
		}
		return err
	}
	if stage != nil {
		return ErrCampaignColumnProtected
	}

	var campaignCount int
	if err = tx.QueryRow(ctx, `SELECT COUNT(*) FROM campaign_column_assignments WHERE column_id = $1::uuid`, columnID).Scan(&campaignCount); err != nil {
		return err
	}
	if campaignCount > 0 {
		return ErrCampaignColumnInUse
	}

	if _, err = tx.Exec(ctx, `DELETE FROM campaign_columns WHERE id = $1::uuid`, columnID); err != nil {
		return err
	}

	maxPosition, err := maxColumnPosition(ctx, tx)
	if err != nil {
		return err
	}
	// Close the gap: every lane after the deleted one moves one step left.
	if err = shiftColumnPositions(ctx, tx, maxPosition, `position > $2`, position, -1); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ReorderColumns sets the lane order. columnIDs must list every lane of the
// board exactly once (ErrCampaignColumnReorderInvalid otherwise); that is
// checked before anything is written.
func (r *CampaignsRepository) ReorderColumns(ctx context.Context, columnIDs []string) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	requested := make(map[string]struct{}, len(columnIDs))
	for _, columnID := range columnIDs {
		key := strings.ToLower(strings.TrimSpace(columnID))
		if _, exists := requested[key]; exists {
			return ErrCampaignColumnReorderInvalid
		}
		requested[key] = struct{}{}
	}

	tx, err := repository.DB(ctx, r.db).Begin(ctx)
	if err != nil {
		return err
	}
	defer rollbackTx(tx)

	// Under the layout lock no lane can be added or removed between the
	// check below and the writes.
	if err = lockCampaignColumnLayout(ctx, tx); err != nil {
		return err
	}

	rows, err := tx.Query(ctx, `SELECT id::text FROM campaign_columns`)
	if err != nil {
		return err
	}
	existing := 0
	for rows.Next() {
		var columnID string
		if err := rows.Scan(&columnID); err != nil {
			rows.Close()
			return err
		}
		if _, ok := requested[columnID]; !ok {
			rows.Close()
			return ErrCampaignColumnReorderInvalid
		}
		existing++
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if existing != len(requested) {
		return ErrCampaignColumnReorderInvalid
	}

	// (tenant_id, position) is unique and checked row by row, so park every
	// lane on a negative position first and then set the final ones.
	for _, final := range []bool{false, true} {
		for index, columnID := range columnIDs {
			position := -(index + 1)
			if final {
				position = index + 1
			}
			tag, err := tx.Exec(ctx, `UPDATE campaign_columns SET position = $2 WHERE id = $1::uuid`, columnID, position)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				// The check above makes this unreachable; kept as a guard.
				return ErrCampaignColumnNotFound
			}
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

// ListActivities is the campaign's activity feed: one entry per action. A
// move and an upload are each written twice to audit_logs (the request audit
// row "move" / "upload_attachments" and the descriptive row "campaign_moved" /
// "attachment_uploaded"); the feed shows the descriptive one. The upload rows
// are written in the upload's transaction, so they always exist. The
// "campaign_moved" row is written after the move commits and is best effort,
// so a "move" row is hidden only when the same user's "campaign_moved" row for
// this campaign was written just before it (the handler writes "move" right
// after the service returns); otherwise the move would vanish from the feed.
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
				WHEN audit_logs.action = 'update'
					AND audit_logs.old_value->>'status' IS NOT NULL
					AND audit_logs.new_value->>'status' IS NOT NULL
					AND audit_logs.old_value->>'status' <> audit_logs.new_value->>'status'
					THEN CONCAT('Updated and moved to ', INITCAP(REPLACE(audit_logs.new_value->>'status', '_', ' ')))
				WHEN audit_logs.action = 'move' THEN 'Moved to another lane'
				ELSE REPLACE(INITCAP(REPLACE(audit_logs.action, '_', ' ')), '  ', ' ')
			END AS description,
			audit_logs.user_id::text,
			users.full_name,
			audit_logs.created_at
		FROM audit_logs
		LEFT JOIN users ON users.id = audit_logs.user_id
		WHERE audit_logs.module = 'marketing' AND audit_logs.resource = 'campaign' AND audit_logs.resource_id = $1
			AND audit_logs.action <> 'upload_attachments'
			AND NOT (
				audit_logs.action = 'move'
				AND EXISTS (
					SELECT 1
					FROM audit_logs AS described
					WHERE described.module = 'marketing'
						AND described.resource = 'campaign'
						AND described.resource_id = audit_logs.resource_id
						AND described.action = 'campaign_moved'
						AND described.user_id IS NOT DISTINCT FROM audit_logs.user_id
						AND described.created_at BETWEEN audit_logs.created_at - INTERVAL '10 seconds' AND audit_logs.created_at
				)
			)
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

// moveCampaignWithinTx places the card in destinationColumnID (at the end when
// requestedPosition is nil) and sets the campaign's status to *status. A nil
// status (custom lane) keeps the status the campaign has.
func (r *CampaignsRepository) moveCampaignWithinTx(ctx context.Context, tx pgx.Tx, campaignID string, destinationColumnID string, requestedPosition *int, movedBy string, status *string) error {
	// Lock order, the same for every writer: the campaign, then the lanes.
	// The campaign lock keeps its placement stable while it is read here.
	if err := r.lockCampaign(ctx, tx, campaignID); err != nil {
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

	lanes := []string{destinationColumnID}
	if currentAssigned && currentColumnID != destinationColumnID {
		lanes = append(lanes, currentColumnID)
	}
	if err := lockColumnsForPlacement(ctx, tx, lanes...); err != nil {
		return err
	}
	// The requested position is an index into the lane as the board shows
	// it. Deleting a campaign leaves a gap in its lane, so number the lanes
	// 1..n first; otherwise "after the last card" can land before it.
	if err := compactLanePositions(ctx, tx, lanes...); err != nil {
		return err
	}
	if currentAssigned {
		// Read the position again under the lane lock: a concurrent move in
		// the same lane may have shifted this card in the meantime.
		if err := tx.QueryRow(ctx, `SELECT position FROM campaign_column_assignments WHERE campaign_id = $1::uuid`, campaignID).Scan(&currentPosition); err != nil {
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

	_, err = tx.Exec(ctx, `UPDATE campaigns SET status = COALESCE($2, status), updated_at = NOW() WHERE id = $1::uuid`, campaignID, status)
	return err
}

// lockCampaign locks the campaign row for the rest of the transaction
// (ErrCampaignNotFound when there is none).
func (r *CampaignsRepository) lockCampaign(ctx context.Context, tx queryExecutor, campaignID string) error {
	var exists bool
	err := tx.QueryRow(ctx, `SELECT TRUE FROM campaigns WHERE id = $1::uuid FOR UPDATE`, campaignID).Scan(&exists)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCampaignNotFound
		}
		return err
	}
	return nil
}

// lockColumnsForPlacement serialises the writers that compute card positions
// in the given lanes. Without it two requests placing a card in the same lane
// at the same time both read the same "last position" and end up sharing it.
// The lanes are locked in id order, so two moves between the same pair of
// lanes in opposite directions cannot deadlock. FOR NO KEY UPDATE is enough:
// it excludes other placements (and a delete of the lane) but not the
// foreign-key checks of unrelated inserts.
func lockColumnsForPlacement(ctx context.Context, tx pgx.Tx, columnIDs ...string) error {
	rows, err := tx.Query(ctx, `
		SELECT id FROM campaign_columns
		WHERE id = ANY($1::uuid[])
		ORDER BY id
		FOR NO KEY UPDATE
	`, columnIDs)
	if err != nil {
		return err
	}
	rows.Close()
	return rows.Err()
}

// compactLanePositions renumbers the cards of the given lanes 1..n in the
// order the board shows them (position, then campaign creation). Only cards
// whose position changes are written, and moved_at/moved_by are left alone:
// the order stays the same, nothing is moved. The caller holds the lane locks.
func compactLanePositions(ctx context.Context, tx pgx.Tx, columnIDs ...string) error {
	_, err := tx.Exec(ctx, `
		UPDATE campaign_column_assignments AS assignment
		SET position = ordered.position
		FROM (
			SELECT
				campaign_column_assignments.campaign_id,
				ROW_NUMBER() OVER (
					PARTITION BY campaign_column_assignments.column_id
					ORDER BY campaign_column_assignments.position, campaigns.created_at, campaigns.id
				)::int AS position
			FROM campaign_column_assignments
			INNER JOIN campaigns ON campaigns.id = campaign_column_assignments.campaign_id
			WHERE campaign_column_assignments.column_id = ANY($1::uuid[])
		) AS ordered
		WHERE assignment.campaign_id = ordered.campaign_id
			AND assignment.position <> ordered.position
	`, columnIDs)
	return err
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

func maxColumnPosition(ctx context.Context, tx queryExecutor) (int, error) {
	var maxPosition int
	err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(position), 0) FROM campaign_columns`).Scan(&maxPosition)
	return maxPosition, err
}

// shiftColumnPositions adds delta to the position of every lane matching
// where ($2 = pivot). (tenant_id, position) is unique and not deferrable, so
// Postgres checks it row by row and a plain "position = position + 1" collides
// with the neighbour that has not moved yet. The lanes are therefore first
// lifted above every position in use ($1 = offset) and then brought down to
// their final value; neither step can meet an occupied position.
func shiftColumnPositions(ctx context.Context, tx pgx.Tx, maxPosition int, where string, pivot int, delta int) error {
	offset := max(maxPosition, 0) + 2
	if _, err := tx.Exec(ctx, `UPDATE campaign_columns SET position = position + $1 WHERE `+where, offset, pivot); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE campaign_columns SET position = position - $1 + $3 WHERE position > $2`, offset, maxPosition+1, delta)
	return err
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

// mapCampaignDBError turns the constraint violations a caller can cause into
// typed errors, so they are answered with 4xx instead of 500.
func mapCampaignDBError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}

	switch pgErr.ConstraintName {
	case "campaigns_pic_employee_id_fkey":
		return ErrCampaignPICNotFound
	case "uq_campaign_columns_tenant_name":
		return ErrCampaignColumnNameTaken
	case "chk_campaigns_date_range":
		return ErrCampaignDateRange
	case "chk_campaigns_channel":
		return ErrCampaignChannelInvalid
	}

	return err
}

func isForeignKeyError(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}
