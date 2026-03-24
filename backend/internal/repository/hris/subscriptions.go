package hris

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kana-consultant/kantor/backend/internal/model"
	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

var (
	ErrSubscriptionNotFound = errors.New("subscription not found")
	ErrAlertNotFound        = errors.New("subscription alert not found")
)

type SubscriptionsRepository struct {
	db repository.DBTX
}

type CreateSubscriptionParams struct {
	Name                      string
	Vendor                    string
	Description               *string
	CostAmount                int64
	CostCurrency              string
	BillingCycle              string
	StartDate                 time.Time
	RenewalDate               time.Time
	Status                    string
	PICEmployeeID             *string
	Category                  string
	LoginCredentialsEncrypted *string
	Notes                     *string
	CreatedBy                 string
}

type UpdateSubscriptionParams = CreateSubscriptionParams

func NewSubscriptionsRepository(db repository.DBTX) *SubscriptionsRepository {
	return &SubscriptionsRepository{db: db}
}

func (r *SubscriptionsRepository) CreateSubscription(ctx context.Context, params CreateSubscriptionParams) (model.Subscription, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	query := `
		INSERT INTO subscriptions (
			name, vendor, description, cost_amount, cost_currency, billing_cycle,
			start_date, renewal_date, status, pic_employee_id, category, login_credentials_encrypted, notes, created_by
		)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, $7::date, $8::date, $9, $10::uuid, $11, NULLIF($12, ''), NULLIF($13, ''), $14::uuid)
		RETURNING id::text
	`

	var subscriptionID string
	err := repository.DB(ctx, r.db).QueryRow(
		ctx,
		query,
		params.Name,
		params.Vendor,
		nullableString(params.Description),
		params.CostAmount,
		params.CostCurrency,
		params.BillingCycle,
		params.StartDate,
		params.RenewalDate,
		params.Status,
		nullableUUID(params.PICEmployeeID),
		params.Category,
		nullableString(params.LoginCredentialsEncrypted),
		nullableString(params.Notes),
		params.CreatedBy,
	).Scan(&subscriptionID)
	if err != nil {
		return model.Subscription{}, err
	}

	return r.GetSubscriptionByID(ctx, subscriptionID)
}

func (r *SubscriptionsRepository) ListSubscriptions(ctx context.Context) ([]model.Subscription, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT
			subscriptions.id::text,
			subscriptions.name,
			subscriptions.vendor,
			subscriptions.description,
			subscriptions.cost_amount,
			subscriptions.cost_currency,
			subscriptions.billing_cycle,
			subscriptions.start_date,
			subscriptions.renewal_date,
			subscriptions.status,
			subscriptions.pic_employee_id::text,
			employees.full_name,
			subscriptions.category,
			subscriptions.login_credentials_encrypted,
			subscriptions.notes,
			subscriptions.created_by::text,
			subscriptions.created_at,
			subscriptions.updated_at
		FROM subscriptions
		LEFT JOIN employees ON employees.id = subscriptions.pic_employee_id
		ORDER BY subscriptions.renewal_date ASC, subscriptions.name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]model.Subscription, 0)
	for rows.Next() {
		subscription, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, subscription)
	}
	return result, rows.Err()
}

func (r *SubscriptionsRepository) GetSubscriptionByID(ctx context.Context, subscriptionID string) (model.Subscription, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	row := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT
			subscriptions.id::text,
			subscriptions.name,
			subscriptions.vendor,
			subscriptions.description,
			subscriptions.cost_amount,
			subscriptions.cost_currency,
			subscriptions.billing_cycle,
			subscriptions.start_date,
			subscriptions.renewal_date,
			subscriptions.status,
			subscriptions.pic_employee_id::text,
			employees.full_name,
			subscriptions.category,
			subscriptions.login_credentials_encrypted,
			subscriptions.notes,
			subscriptions.created_by::text,
			subscriptions.created_at,
			subscriptions.updated_at
		FROM subscriptions
		LEFT JOIN employees ON employees.id = subscriptions.pic_employee_id
		WHERE subscriptions.id = $1::uuid
	`, subscriptionID)

	subscription, err := scanSubscription(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Subscription{}, ErrSubscriptionNotFound
	}
	return subscription, err
}

func (r *SubscriptionsRepository) UpdateSubscription(ctx context.Context, subscriptionID string, params UpdateSubscriptionParams) (model.Subscription, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tag, err := repository.DB(ctx, r.db).Exec(ctx, `
		UPDATE subscriptions
		SET
			name = $2,
			vendor = $3,
			description = NULLIF($4, ''),
			cost_amount = $5,
			cost_currency = $6,
			billing_cycle = $7,
			start_date = $8::date,
			renewal_date = $9::date,
			status = $10,
			pic_employee_id = $11::uuid,
			category = $12,
			login_credentials_encrypted = NULLIF($13, ''),
			notes = NULLIF($14, ''),
			updated_at = NOW()
		WHERE id = $1::uuid
	`, subscriptionID, params.Name, params.Vendor, nullableString(params.Description), params.CostAmount, params.CostCurrency, params.BillingCycle, params.StartDate, params.RenewalDate, params.Status, nullableUUID(params.PICEmployeeID), params.Category, nullableString(params.LoginCredentialsEncrypted), nullableString(params.Notes))
	if err != nil {
		return model.Subscription{}, err
	}
	if tag.RowsAffected() == 0 {
		return model.Subscription{}, ErrSubscriptionNotFound
	}
	return r.GetSubscriptionByID(ctx, subscriptionID)
}

func (r *SubscriptionsRepository) DeleteSubscription(ctx context.Context, subscriptionID string) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tag, err := repository.DB(ctx, r.db).Exec(ctx, `DELETE FROM subscriptions WHERE id = $1::uuid`, subscriptionID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrSubscriptionNotFound
	}
	return nil
}

func (r *SubscriptionsRepository) ListSubscriptionsForAlertCheck(ctx context.Context) ([]model.Subscription, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT
			subscriptions.id::text,
			subscriptions.name,
			subscriptions.vendor,
			subscriptions.description,
			subscriptions.cost_amount,
			subscriptions.cost_currency,
			subscriptions.billing_cycle,
			subscriptions.start_date,
			subscriptions.renewal_date,
			subscriptions.status,
			subscriptions.pic_employee_id::text,
			employees.full_name,
			subscriptions.category,
			subscriptions.login_credentials_encrypted,
			subscriptions.notes,
			subscriptions.created_by::text,
			subscriptions.created_at,
			subscriptions.updated_at
		FROM subscriptions
		LEFT JOIN employees ON employees.id = subscriptions.pic_employee_id
		WHERE subscriptions.status = 'active'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]model.Subscription, 0)
	for rows.Next() {
		subscription, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, subscription)
	}
	return result, rows.Err()
}

func (r *SubscriptionsRepository) AlertExistsForDay(ctx context.Context, subscriptionID string, alertType string, day time.Time) (bool, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var exists bool
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM subscription_alerts
			WHERE subscription_id = $1::uuid
			  AND alert_type = $2
			  AND DATE(created_at) = $3::date
		)
	`, subscriptionID, alertType, day).Scan(&exists)
	return exists, err
}

func (r *SubscriptionsRepository) CreateSubscriptionAlert(ctx context.Context, subscriptionID string, alertType string) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	_, err := repository.DB(ctx, r.db).Exec(ctx, `
		INSERT INTO subscription_alerts (subscription_id, alert_type)
		VALUES ($1::uuid, $2)
	`, subscriptionID, alertType)
	return err
}

func (r *SubscriptionsRepository) ListAlerts(ctx context.Context) ([]model.SubscriptionAlert, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT
			subscription_alerts.id::text,
			subscription_alerts.subscription_id::text,
			subscriptions.name,
			subscription_alerts.alert_type,
			subscription_alerts.is_read,
			subscription_alerts.created_at
		FROM subscription_alerts
		INNER JOIN subscriptions ON subscriptions.id = subscription_alerts.subscription_id
		ORDER BY subscription_alerts.is_read ASC, subscription_alerts.created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]model.SubscriptionAlert, 0)
	for rows.Next() {
		var alert model.SubscriptionAlert
		if err := rows.Scan(
			&alert.ID,
			&alert.SubscriptionID,
			&alert.SubscriptionName,
			&alert.AlertType,
			&alert.IsRead,
			&alert.CreatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, alert)
	}
	return result, rows.Err()
}

func (r *SubscriptionsRepository) MarkAlertRead(ctx context.Context, alertID string) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tag, err := repository.DB(ctx, r.db).Exec(ctx, `UPDATE subscription_alerts SET is_read = TRUE WHERE id = $1::uuid`, alertID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrAlertNotFound
	}
	return nil
}

func (r *SubscriptionsRepository) Summary(ctx context.Context) (model.SubscriptionSummary, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT category, cost_amount, billing_cycle, status
		FROM subscriptions
		WHERE status = 'active'
	`)
	if err != nil {
		return model.SubscriptionSummary{}, err
	}
	defer rows.Close()

	summary := model.SubscriptionSummary{
		ByCategory: map[string]int64{},
	}
	for rows.Next() {
		var category string
		var amount int64
		var billingCycle string
		var status string
		if err := rows.Scan(&category, &amount, &billingCycle, &status); err != nil {
			return model.SubscriptionSummary{}, err
		}

		summary.ActiveCount++
		summary.ByCategory[category] += amount
		summary.TotalMonthlyCost += monthlyCost(amount, billingCycle)
		summary.TotalYearlyCost += yearlyCost(amount, billingCycle)
	}
	return summary, rows.Err()
}

func monthlyCost(amount int64, billingCycle string) int64 {
	switch billingCycle {
	case "monthly":
		return amount
	case "quarterly":
		return amount / 3
	case "yearly":
		return amount / 12
	default:
		return 0
	}
}

func yearlyCost(amount int64, billingCycle string) int64 {
	switch billingCycle {
	case "monthly":
		return amount * 12
	case "quarterly":
		return amount * 4
	case "yearly":
		return amount
	default:
		return 0
	}
}

type scanner interface {
	Scan(dest ...any) error
}

func scanSubscription(row scanner) (model.Subscription, error) {
	var subscription model.Subscription
	err := row.Scan(
		&subscription.ID,
		&subscription.Name,
		&subscription.Vendor,
		&subscription.Description,
		&subscription.CostAmount,
		&subscription.CostCurrency,
		&subscription.BillingCycle,
		&subscription.StartDate,
		&subscription.RenewalDate,
		&subscription.Status,
		&subscription.PICEmployeeID,
		&subscription.PICEmployeeName,
		&subscription.Category,
		&subscription.LoginCredentialsPlain,
		&subscription.Notes,
		&subscription.CreatedBy,
		&subscription.CreatedAt,
		&subscription.UpdatedAt,
	)
	return subscription, err
}
