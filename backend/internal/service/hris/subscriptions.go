package hris

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/dto/hris"
	"github.com/kana-consultant/kantor/backend/internal/model"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
	"github.com/kana-consultant/kantor/backend/internal/security"
)

var (
	ErrSubscriptionNotFound      = errors.New("subscription not found")
	ErrSubscriptionAlertNotFound = errors.New("subscription alert not found")
)

type subscriptionsRepository interface {
	CreateSubscription(ctx context.Context, params hrisrepo.CreateSubscriptionParams) (model.Subscription, error)
	ListSubscriptions(ctx context.Context) ([]model.Subscription, error)
	GetSubscriptionByID(ctx context.Context, subscriptionID string) (model.Subscription, error)
	UpdateSubscription(ctx context.Context, subscriptionID string, params hrisrepo.UpdateSubscriptionParams) (model.Subscription, error)
	DeleteSubscription(ctx context.Context, subscriptionID string) error
	Summary(ctx context.Context) (model.SubscriptionSummary, error)
	ListAlerts(ctx context.Context) ([]model.SubscriptionAlert, error)
	MarkAlertRead(ctx context.Context, alertID string) error
	ListSubscriptionsForAlertCheck(ctx context.Context) ([]model.Subscription, error)
	AlertExistsForDay(ctx context.Context, subscriptionID string, alertType string, day time.Time) (bool, error)
	CreateSubscriptionAlert(ctx context.Context, subscriptionID string, alertType string) error
}

type subscriptionsEmployeesRepository interface {
	GetEmployeeByID(ctx context.Context, employeeID string) (model.Employee, error)
}

type SubscriptionsService struct {
	repo           subscriptionsRepository
	employeesRepo  subscriptionsEmployeesRepository
	encrypter      *security.Encrypter
	financeService *FinanceService
}

func NewSubscriptionsService(repo subscriptionsRepository, employeesRepo subscriptionsEmployeesRepository, encrypter *security.Encrypter, financeService *FinanceService) *SubscriptionsService {
	return &SubscriptionsService{
		repo:           repo,
		employeesRepo:  employeesRepo,
		encrypter:      encrypter,
		financeService: financeService,
	}
}

func (s *SubscriptionsService) CreateSubscription(ctx context.Context, request hris.CreateSubscriptionRequest, actorID string) (model.Subscription, error) {
	params, err := s.mapParams(ctx, request, actorID)
	if err != nil {
		return model.Subscription{}, err
	}
	subscription, err := s.repo.CreateSubscription(ctx, params)
	if err != nil {
		return model.Subscription{}, mapSubscriptionError(err)
	}

	if s.financeService != nil && subscription.Status == "active" {
		desc := "Subscription: " + subscription.Name + " (" + subscription.Vendor + ")"
		if finErr := s.financeService.RecordOutcome(ctx, "subscription", subscription.CostAmount, desc, subscription.StartDate, actorID); finErr != nil {
			return model.Subscription{}, fmt.Errorf("record finance entry: %w", finErr)
		}
	}

	return s.decryptSubscription(subscription)
}

func (s *SubscriptionsService) ListSubscriptions(ctx context.Context) ([]model.Subscription, error) {
	if err := s.GenerateSubscriptionAlerts(ctx, time.Now()); err != nil {
		return nil, err
	}
	items, err := s.repo.ListSubscriptions(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]model.Subscription, 0, len(items))
	for _, item := range items {
		decrypted, err := s.decryptSubscription(item)
		if err != nil {
			return nil, err
		}
		result = append(result, decrypted)
	}
	return result, nil
}

func (s *SubscriptionsService) GetSubscription(ctx context.Context, subscriptionID string) (model.Subscription, error) {
	item, err := s.repo.GetSubscriptionByID(ctx, subscriptionID)
	if err != nil {
		return model.Subscription{}, mapSubscriptionError(err)
	}
	return s.decryptSubscription(item)
}

func (s *SubscriptionsService) UpdateSubscription(ctx context.Context, subscriptionID string, request hris.UpdateSubscriptionRequest, actorID string) (model.Subscription, error) {
	params, err := s.mapParams(ctx, request, actorID)
	if err != nil {
		return model.Subscription{}, err
	}
	item, err := s.repo.UpdateSubscription(ctx, subscriptionID, params)
	if err != nil {
		return model.Subscription{}, mapSubscriptionError(err)
	}
	return s.decryptSubscription(item)
}

func (s *SubscriptionsService) DeleteSubscription(ctx context.Context, subscriptionID string) error {
	return mapSubscriptionError(s.repo.DeleteSubscription(ctx, subscriptionID))
}

func (s *SubscriptionsService) Summary(ctx context.Context) (model.SubscriptionSummary, error) {
	if err := s.GenerateSubscriptionAlerts(ctx, time.Now()); err != nil {
		return model.SubscriptionSummary{}, err
	}
	return s.repo.Summary(ctx)
}

func (s *SubscriptionsService) ListAlerts(ctx context.Context) ([]model.SubscriptionAlert, error) {
	if err := s.GenerateSubscriptionAlerts(ctx, time.Now()); err != nil {
		return nil, err
	}
	return s.repo.ListAlerts(ctx)
}

func (s *SubscriptionsService) MarkAlertRead(ctx context.Context, alertID string) error {
	err := s.repo.MarkAlertRead(ctx, alertID)
	if errors.Is(err, hrisrepo.ErrAlertNotFound) {
		return ErrSubscriptionAlertNotFound
	}
	return err
}

func (s *SubscriptionsService) GenerateSubscriptionAlerts(ctx context.Context, now time.Time) error {
	subscriptions, err := s.repo.ListSubscriptionsForAlertCheck(ctx)
	if err != nil {
		return err
	}

	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	for _, subscription := range subscriptions {
		daysUntilRenewal := int(subscription.RenewalDate.Sub(today).Hours() / 24)
		alertType := ""
		switch daysUntilRenewal {
		case 30:
			alertType = "30_days"
		case 7:
			alertType = "7_days"
		case 1:
			alertType = "1_day"
		default:
			continue
		}

		exists, err := s.repo.AlertExistsForDay(ctx, subscription.ID, alertType, today)
		if err != nil {
			return err
		}
		if exists {
			continue
		}

		if err := s.repo.CreateSubscriptionAlert(ctx, subscription.ID, alertType); err != nil {
			return err
		}
	}

	return nil
}

func (s *SubscriptionsService) mapParams(ctx context.Context, request hris.CreateSubscriptionRequest, actorID string) (hrisrepo.CreateSubscriptionParams, error) {
	if request.PICEmployeeID != nil && strings.TrimSpace(*request.PICEmployeeID) != "" {
		if _, err := s.employeesRepo.GetEmployeeByID(ctx, strings.TrimSpace(*request.PICEmployeeID)); err != nil {
			return hrisrepo.CreateSubscriptionParams{}, err
		}
	}

	var credentialsEncrypted *string
	if request.LoginCredentials != nil && strings.TrimSpace(*request.LoginCredentials) != "" {
		ciphertext, err := s.encrypter.EncryptString(strings.TrimSpace(*request.LoginCredentials))
		if err != nil {
			return hrisrepo.CreateSubscriptionParams{}, err
		}
		credentialsEncrypted = &ciphertext
	}

	return hrisrepo.CreateSubscriptionParams{
		Name:                      strings.TrimSpace(request.Name),
		Vendor:                    strings.TrimSpace(request.Vendor),
		Description:               trimOptionalString(request.Description),
		CostAmount:                request.CostAmount,
		CostCurrency:              strings.ToUpper(strings.TrimSpace(request.CostCurrency)),
		BillingCycle:              strings.TrimSpace(request.BillingCycle),
		StartDate:                 request.StartDate,
		RenewalDate:               request.RenewalDate,
		Status:                    strings.TrimSpace(request.Status),
		PICEmployeeID:             trimOptionalString(request.PICEmployeeID),
		Category:                  strings.TrimSpace(request.Category),
		LoginCredentialsEncrypted: credentialsEncrypted,
		Notes:                     trimOptionalString(request.Notes),
		CreatedBy:                 actorID,
	}, nil
}

func (s *SubscriptionsService) decryptSubscription(item model.Subscription) (model.Subscription, error) {
	if item.LoginCredentialsPlain == nil || strings.TrimSpace(*item.LoginCredentialsPlain) == "" {
		return item, nil
	}

	decrypted, err := s.encrypter.DecryptString(*item.LoginCredentialsPlain)
	if err != nil {
		return model.Subscription{}, err
	}
	item.LoginCredentialsPlain = &decrypted
	return item, nil
}

func mapSubscriptionError(err error) error {
	switch {
	case errors.Is(err, hrisrepo.ErrSubscriptionNotFound):
		return ErrSubscriptionNotFound
	case errors.Is(err, hrisrepo.ErrEmployeeNotFound):
		return ErrEmployeeNotFound
	default:
		return err
	}
}
