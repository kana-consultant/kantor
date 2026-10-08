package marketing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kana-consultant/kantor/backend/internal/model"
	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

// ErrCampaignStageUnavailable is matched (errors.Is) by
// *CampaignStageUnavailableError.
var ErrCampaignStageUnavailable = errors.New("campaign stage has no board column")

// CampaignStageUnavailableError says the board has no column for a stage even
// after the stage columns were ensured inside the same transaction. Cause is
// what stopped the ensure, when something did.
type CampaignStageUnavailableError struct {
	Stage string
	Cause error
}

func (e *CampaignStageUnavailableError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("campaign stage %q has no board column: %v", e.Stage, e.Cause)
	}
	return fmt.Sprintf("campaign stage %q has no board column", e.Stage)
}

func (e *CampaignStageUnavailableError) Is(target error) bool {
	return target == ErrCampaignStageUnavailable
}

func (e *CampaignStageUnavailableError) Unwrap() error { return e.Cause }

// stageColumnDefault is the column appended for a stage no column carries.
type stageColumnDefault struct {
	Stage string
	Name  string
	Color string
}

// defaultStageColumns lists the six campaign stages in board order with the
// name and colour the first migration gave their columns. The stage keys are
// the values of campaigns.status (dto/marketing.CampaignStages).
var defaultStageColumns = []stageColumnDefault{
	{Stage: "ideation", Name: "Ideation", Color: "#8B5CF6"},
	{Stage: "planning", Name: "Planning", Color: "#0EA5E9"},
	{Stage: "in_production", Name: "In Production", Color: "#F59E0B"},
	{Stage: "live", Name: "Live", Color: "#10B981"},
	{Stage: "completed", Name: "Completed", Color: "#334155"},
	{Stage: "archived", Name: "Archived", Color: "#94A3B8"},
}

// EnsureStageColumnsResult lists the stages a call had to repair.
type EnsureStageColumnsResult struct {
	// Adopted: an existing column whose name already matched the stage was
	// given the stage key. Nothing else about that column changed.
	Adopted []string
	// Created: no such column existed, so the default one was appended after
	// the last column.
	Created []string
}

// Changed reports whether the call wrote anything.
func (r EnsureStageColumnsResult) Changed() bool {
	return len(r.Adopted)+len(r.Created) > 0
}

// EnsureStageColumns guarantees that the current tenant's board has one
// column for each of the six campaign stages. It is idempotent and only ever
//
//   - sets `stage` on an existing column that has none and whose name
//     canonicalises to the stage (the first by position wins), or
//   - appends the default column for the stage after the last column.
//
// It never deletes, renames, reorders or recolours a column and never touches
// a campaign or its placement. Each stage is repaired on its own (a
// savepoint), so one stage that cannot be repaired does not undo the others;
// the returned error then joins the per-stage failures while the result still
// lists what was done.
//
// Called at boot for every tenant (best effort) and, through findStageColumn,
// inside the create/update transaction when a stage has no column.
func (r *CampaignsRepository) EnsureStageColumns(ctx context.Context) (EnsureStageColumnsResult, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tx, err := repository.DB(ctx, r.db).Begin(ctx)
	if err != nil {
		return EnsureStageColumnsResult{}, err
	}
	defer rollbackTx(tx)

	result, ensureErr := r.ensureStageColumnsTx(ctx, tx)
	if !result.Changed() {
		// Nothing to keep: the deferred rollback ends the transaction.
		return result, ensureErr
	}
	if err := tx.Commit(ctx); err != nil {
		return EnsureStageColumnsResult{}, errors.Join(ensureErr, err)
	}
	return result, ensureErr
}

type stageColumnRow struct {
	id    string
	name  string
	stage *string
}

// ensureStageColumnsTx is EnsureStageColumns inside the caller's transaction.
// Every statement runs in a savepoint, so a failure here never leaves tx
// aborted: the caller can go on using it.
func (r *CampaignsRepository) ensureStageColumnsTx(ctx context.Context, tx pgx.Tx) (EnsureStageColumnsResult, error) {
	var (
		result  EnsureStageColumnsResult
		columns []stageColumnRow
	)

	err := inSavepoint(ctx, tx, func(sp pgx.Tx) error {
		// One repair at a time per tenant: two requests (or two server
		// instances booting) must not both append the same stage column.
		if err := lockCampaignColumnLayout(ctx, sp); err != nil {
			return err
		}

		rows, err := sp.Query(ctx, `
			SELECT id::text, name, stage
			FROM campaign_columns
			ORDER BY position ASC, created_at ASC, id ASC
		`)
		if err != nil {
			return err
		}
		defer rows.Close()

		columns = columns[:0]
		for rows.Next() {
			var row stageColumnRow
			if err := rows.Scan(&row.id, &row.name, &row.stage); err != nil {
				return err
			}
			columns = append(columns, row)
		}
		return rows.Err()
	})
	if err != nil {
		return result, fmt.Errorf("load campaign columns: %w", err)
	}

	staged := make(map[string]bool, len(defaultStageColumns))
	names := make(map[string]bool, len(columns))
	for _, column := range columns {
		names[column.name] = true
		if column.stage != nil {
			staged[*column.stage] = true
		}
	}

	var failures []error
	for _, def := range defaultStageColumns {
		if staged[def.Stage] {
			continue
		}

		// Adopt: the first column by position that has no stage yet and is
		// named like this stage. Later columns with such a name stay custom.
		adopt := -1
		for index, column := range columns {
			if column.stage == nil && canonicalCampaignState(column.name) == def.Stage {
				adopt = index
				break
			}
		}
		if adopt >= 0 {
			columnID := columns[adopt].id
			err := inSavepoint(ctx, tx, func(sp pgx.Tx) error {
				tag, err := sp.Exec(ctx, `UPDATE campaign_columns SET stage = $2 WHERE id = $1::uuid AND stage IS NULL`, columnID, def.Stage)
				if err != nil {
					return err
				}
				if tag.RowsAffected() != 1 {
					return fmt.Errorf("column %s was changed concurrently", columnID)
				}
				return nil
			})
			if err != nil {
				failures = append(failures, fmt.Errorf("adopt column for stage %s: %w", def.Stage, err))
				continue
			}
			stage := def.Stage
			columns[adopt].stage = &stage
			staged[def.Stage] = true
			result.Adopted = append(result.Adopted, def.Stage)
			continue
		}

		// Append: after the last column, under a name no column uses yet
		// (a column called "Live" may already stand for another stage). The
		// last position is read by the INSERT itself, so a column another
		// writer committed in the meantime cannot make every later append
		// collide on the same position.
		name := unusedColumnName(def.Name, names)
		err := inSavepoint(ctx, tx, func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `
				INSERT INTO campaign_columns (name, position, color, stage)
				SELECT $1, COALESCE(MAX(position), 0) + 1, $2, $3
				FROM campaign_columns
			`, name, def.Color, def.Stage)
			return err
		})
		if err != nil {
			failures = append(failures, fmt.Errorf("append column for stage %s: %w", def.Stage, err))
			continue
		}
		names[name] = true
		staged[def.Stage] = true
		result.Created = append(result.Created, def.Stage)
	}

	return result, errors.Join(failures...)
}

// findStageColumn returns the column that carries stage. When the tenant has
// none it ensures the stage columns inside tx and looks again; if the stage
// is still missing the error is a *CampaignStageUnavailableError.
func (r *CampaignsRepository) findStageColumn(ctx context.Context, tx pgx.Tx, stage string) (model.CampaignColumn, error) {
	column, err := selectStageColumn(ctx, tx, stage)
	if err == nil || !errors.Is(err, pgx.ErrNoRows) {
		return column, err
	}

	_, ensureErr := r.ensureStageColumnsTx(ctx, tx)

	column, err = selectStageColumn(ctx, tx, stage)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.CampaignColumn{}, &CampaignStageUnavailableError{Stage: stage, Cause: ensureErr}
	}
	return column, err
}

func selectStageColumn(ctx context.Context, tx queryExecutor, stage string) (model.CampaignColumn, error) {
	var item model.CampaignColumn
	err := tx.QueryRow(ctx, `
		SELECT id::text, name, position, color, stage, created_at
		FROM campaign_columns
		WHERE stage = $1
	`, stage).Scan(&item.ID, &item.Name, &item.Position, &item.Color, &item.Stage, &item.CreatedAt)
	return item, err
}

// stageForColumnID returns the stage a column stands for, nil for a custom
// column.
func (r *CampaignsRepository) stageForColumnID(ctx context.Context, tx queryExecutor, columnID string) (*string, error) {
	var stage *string
	err := tx.QueryRow(ctx, `SELECT stage FROM campaign_columns WHERE id = $1::uuid`, columnID).Scan(&stage)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCampaignColumnNotFound
		}
		return nil, err
	}
	return stage, nil
}

// lockCampaignColumnLayout serialises, per tenant, every writer that adds,
// removes or re-positions board columns: the stage guarantee, CreateColumn,
// DeleteColumn and ReorderColumns. Each of them reads the positions in use and
// then writes new ones, and (tenant_id, position) is unique, so two of them
// running side by side would collide. Every caller takes this lock before it
// locks any column row. It is released when the transaction ends.
func lockCampaignColumnLayout(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `
		SELECT pg_advisory_xact_lock(
			hashtextextended('campaign_stage_columns:' || COALESCE(current_setting('app.current_tenant', true), ''), 0)
		)
	`)
	return err
}

// inSavepoint runs fn in a savepoint of tx: an error rolls back only what fn
// did and leaves tx usable.
func inSavepoint(ctx context.Context, tx pgx.Tx, fn func(sp pgx.Tx) error) error {
	sp, err := tx.Begin(ctx)
	if err != nil {
		return err
	}
	if err := fn(sp); err != nil {
		rollbackTx(sp)
		return err
	}
	return sp.Commit(ctx)
}

// rollbackTx rolls tx back on a context of its own, so a request context
// that has already expired cannot leave the transaction open. It is a no-op
// after a commit.
func rollbackTx(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}

func unusedColumnName(base string, used map[string]bool) string {
	if !used[base] {
		return base
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s (%d)", base, suffix)
		if !used[candidate] {
			return candidate
		}
	}
}
