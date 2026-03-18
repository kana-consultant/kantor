package notifications

import (
	"context"
	"errors"
	"strings"

	"github.com/kana-consultant/kantor/backend/internal/model"
	notificationsrepo "github.com/kana-consultant/kantor/backend/internal/repository/notifications"
)

var ErrNotificationNotFound = errors.New("notification not found")

type Service struct {
	repo *notificationsrepo.Repository
}

type ListParams struct {
	UserID  string
	Read    *bool
	Page    int
	PerPage int
}

func New(repo *notificationsrepo.Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) CreateMany(ctx context.Context, params []notificationsrepo.CreateParams) error {
	return s.repo.CreateMany(ctx, params)
}

func (s *Service) List(ctx context.Context, params ListParams) ([]model.Notification, int64, error) {
	page := params.Page
	if page < 1 {
		page = 1
	}
	perPage := params.PerPage
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}

	return s.repo.List(ctx, notificationsrepo.ListParams{
		UserID: strings.TrimSpace(params.UserID),
		Read:   params.Read,
		Limit:  perPage,
		Offset: (page - 1) * perPage,
	})
}

func (s *Service) CountUnread(ctx context.Context, userID string) (int64, error) {
	return s.repo.CountUnread(ctx, strings.TrimSpace(userID))
}

func (s *Service) MarkRead(ctx context.Context, notificationID string, userID string) error {
	err := s.repo.MarkRead(ctx, notificationID, userID)
	if errors.Is(err, notificationsrepo.ErrNotificationNotFound) {
		return ErrNotificationNotFound
	}
	return err
}

func (s *Service) MarkAllRead(ctx context.Context, userID string) error {
	return s.repo.MarkAllRead(ctx, userID)
}
