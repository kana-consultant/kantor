package hris

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kana-consultant/kantor/backend/internal/model"
	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

var ErrHRProfileNotFound = errors.New("employee hr profile not found")

type HRProfilesRepository struct {
	db        repository.DBTX
	sequences *DocumentSequencesRepository
}

func NewHRProfilesRepository(db repository.DBTX, sequences *DocumentSequencesRepository) *HRProfilesRepository {
	return &HRProfilesRepository{db: db, sequences: sequences}
}

// UpsertHRProfileParams writes only the groups whose Set flag is true; the
// other columns keep their stored value (or stay NULL on insert). A nil value
// with its Set flag clears the column.
type UpsertHRProfileParams struct {
	EmployeeID        string
	SetJobTitle       bool
	JobTitle          *string
	SetPersonalEmail  bool
	PersonalEmail     *string
	SetIdentity       bool
	IdentityEncrypted *string
	UpdatedBy         string
}

const hrProfileColumns = `
	employee_id::text, employee_number, employee_code, job_title, personal_email,
	identity_encrypted, identity_updated_at, updated_by::text, created_at, updated_at`

func scanHRProfile(row pgx.Row) (model.EmployeeHRProfile, error) {
	var profile model.EmployeeHRProfile
	err := row.Scan(
		&profile.EmployeeID,
		&profile.EmployeeNumber,
		&profile.EmployeeCode,
		&profile.JobTitle,
		&profile.PersonalEmail,
		&profile.IdentityEncrypted,
		&profile.IdentityUpdatedAt,
		&profile.UpdatedBy,
		&profile.CreatedAt,
		&profile.UpdatedAt,
	)
	return profile, err
}

func (r *HRProfilesRepository) GetByEmployeeID(ctx context.Context, employeeID string) (model.EmployeeHRProfile, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	profile, err := scanHRProfile(repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT `+hrProfileColumns+`
		FROM employee_hr_profiles
		WHERE employee_id = $1::uuid
	`, employeeID))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.EmployeeHRProfile{}, ErrHRProfileNotFound
	}
	return profile, err
}

func (r *HRProfilesRepository) Upsert(ctx context.Context, params UpsertHRProfileParams) (model.EmployeeHRProfile, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var jobTitle, personalEmail, identity *string
	if params.SetJobTitle {
		jobTitle = params.JobTitle
	}
	if params.SetPersonalEmail {
		personalEmail = params.PersonalEmail
	}
	if params.SetIdentity {
		identity = params.IdentityEncrypted
	}

	profile, err := scanHRProfile(repository.DB(ctx, r.db).QueryRow(ctx, `
		INSERT INTO employee_hr_profiles (
			employee_id, job_title, personal_email, identity_encrypted, identity_updated_at, updated_by
		)
		VALUES (
			$1::uuid, $3, $5, $7,
			CASE WHEN $6::boolean THEN NOW() END,
			NULLIF($8, '')::uuid
		)
		ON CONFLICT (employee_id) DO UPDATE
		SET job_title = CASE WHEN $2::boolean THEN EXCLUDED.job_title ELSE employee_hr_profiles.job_title END,
		    personal_email = CASE WHEN $4::boolean THEN EXCLUDED.personal_email ELSE employee_hr_profiles.personal_email END,
		    identity_encrypted = CASE WHEN $6::boolean THEN EXCLUDED.identity_encrypted ELSE employee_hr_profiles.identity_encrypted END,
		    identity_updated_at = CASE WHEN $6::boolean THEN NOW() ELSE employee_hr_profiles.identity_updated_at END,
		    updated_by = EXCLUDED.updated_by,
		    updated_at = NOW()
		RETURNING `+hrProfileColumns,
		params.EmployeeID,
		params.SetJobTitle, jobTitle,
		params.SetPersonalEmail, personalEmail,
		params.SetIdentity, identity,
		params.UpdatedBy,
	))
	if err != nil {
		if isForeignKeyViolation(err) {
			return model.EmployeeHRProfile{}, ErrEmployeeNotFound
		}
		return model.EmployeeHRProfile{}, err
	}
	return profile, nil
}

// EnsureEmployeeCode returns the employee's number and code, assigning the
// next 'EMPLOYEE'/'ALL' sequence value (formatted by formatCode) on first
// use. It runs in its own transaction (a savepoint when ctx already carries
// one) and locks the profile row, so concurrent first generations for the
// same employee agree on one number.
func (r *HRProfilesRepository) EnsureEmployeeCode(ctx context.Context, employeeID string, formatCode func(number int) string) (number int, code string, err error) {
	tx, err := repository.DB(ctx, r.db).Begin(ctx)
	if err != nil {
		return 0, "", err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(context.Background())
		}
	}()
	txCtx := repository.WithConn(ctx, tx)

	if _, err = tx.Exec(txCtx, `
		INSERT INTO employee_hr_profiles (employee_id)
		VALUES ($1::uuid)
		ON CONFLICT (employee_id) DO NOTHING
	`, employeeID); err != nil {
		if isForeignKeyViolation(err) {
			err = ErrEmployeeNotFound
		}
		return 0, "", err
	}

	var existingNumber *int
	var existingCode *string
	if err = tx.QueryRow(txCtx, `
		SELECT employee_number, employee_code
		FROM employee_hr_profiles
		WHERE employee_id = $1::uuid
		FOR UPDATE
	`, employeeID).Scan(&existingNumber, &existingCode); err != nil {
		return 0, "", err
	}

	if existingNumber != nil && existingCode != nil {
		number, code = *existingNumber, *existingCode
	} else {
		number, err = r.sequences.Next(txCtx, DocSequenceEmployee, DocSequencePeriodAll)
		if err != nil {
			return 0, "", fmt.Errorf("next employee number: %w", err)
		}
		code = formatCode(number)
		if _, err = tx.Exec(txCtx, `
			UPDATE employee_hr_profiles
			SET employee_number = $2, employee_code = $3, updated_at = NOW()
			WHERE employee_id = $1::uuid
		`, employeeID, number, code); err != nil {
			return 0, "", err
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return 0, "", err
	}
	return number, code, nil
}

// LogIdentityAccess writes an audit_logs row for a read of the identity
// data. Callers treat an error as fatal (fail-closed): no identity data is
// returned unless the access was recorded.
func (r *HRProfilesRepository) LogIdentityAccess(ctx context.Context, userID string, employeeID string, action string) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	_, err := repository.DB(ctx, r.db).Exec(ctx, `
		INSERT INTO audit_logs (user_id, action, module, resource, resource_id, created_at)
		VALUES (NULLIF($1, '')::uuid, $2, 'hris', 'employee_identity', $3, NOW())
	`, userID, action, employeeID)
	return err
}

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}
