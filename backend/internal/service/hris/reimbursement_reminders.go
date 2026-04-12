package hris

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/model"
	notificationsrepo "github.com/kana-consultant/kantor/backend/internal/repository/notifications"
	"github.com/kana-consultant/kantor/backend/internal/tenant"
)

const reimbursementReminderPreviewLimit = 3

func (s *ReimbursementsService) RunReminderJobs(ctx context.Context, now time.Time) error {
	setting, err := s.authRepo.GetReimbursementReminderSetting(ctx)
	if err != nil {
		return err
	}
	if !setting.Enabled {
		return nil
	}

	if setting.Review.Enabled && reminderCronMatches(setting.Review.Cron, now) && s.shouldRunReminder(ctx, "review", now) {
		if err := s.runReminderKind(ctx, "review", "submitted", setting.Review, "hris:reimbursement:approve"); err != nil {
			return err
		}
	}
	if setting.Payment.Enabled && reminderCronMatches(setting.Payment.Cron, now) && s.shouldRunReminder(ctx, "payment", now) {
		if err := s.runReminderKind(ctx, "payment", "approved", setting.Payment, "hris:reimbursement:mark_paid"); err != nil {
			return err
		}
	}

	return nil
}

func (s *ReimbursementsService) runReminderKind(ctx context.Context, kind string, status string, rule model.ReimbursementReminderRule, actionPermission string) error {
	digest, err := s.repo.GetReminderDigest(ctx, status, reimbursementReminderPreviewLimit)
	if err != nil {
		return err
	}
	if digest.PendingCount == 0 {
		return nil
	}

	digest.Kind = kind
	digest.Title = reminderTitle(kind)

	recipients, err := s.resolveReminderRecipients(ctx, actionPermission)
	if err != nil {
		return err
	}
	if len(recipients) == 0 {
		return nil
	}

	if rule.Channels.InApp {
		if err := s.sendReminderInAppNotifications(ctx, recipients, digest); err != nil {
			return err
		}
	}
	if rule.Channels.Email && s.reminderEmailSender != nil {
		for _, recipient := range recipients {
			s.reminderEmailSender.SendReimbursementReminder(ctx, recipient, digest)
		}
	}
	if rule.Channels.WhatsApp && s.reminderWASender != nil {
		for _, recipient := range recipients {
			s.reminderWASender.SendReimbursementReminder(ctx, recipient, digest)
		}
	}

	return nil
}

func (s *ReimbursementsService) resolveReminderRecipients(ctx context.Context, actionPermission string) ([]model.ReimbursementReminderRecipient, error) {
	actionUsers, err := s.authRepo.ListUserIDsByPermission(ctx, actionPermission)
	if err != nil {
		return nil, err
	}
	viewAllUsers, err := s.authRepo.ListUserIDsByPermission(ctx, "hris:reimbursement:view_all")
	if err != nil {
		return nil, err
	}

	resolved := intersectUserIDs(actionUsers, viewAllUsers)
	if len(resolved) == 0 {
		return []model.ReimbursementReminderRecipient{}, nil
	}
	return s.authRepo.ListUserReminderRecipients(ctx, resolved)
}

func (s *ReimbursementsService) sendReminderInAppNotifications(ctx context.Context, recipients []model.ReimbursementReminderRecipient, digest model.ReimbursementReminderDigest) error {
	if s.notificationsService == nil {
		return nil
	}

	title, message := inAppReminderMessage(digest)
	params := make([]notificationsrepo.CreateParams, 0, len(recipients))
	for _, recipient := range recipients {
		if strings.TrimSpace(recipient.UserID) == "" {
			continue
		}
		params = append(params, notificationsrepo.CreateParams{
			UserID:  recipient.UserID,
			Type:    "reimbursement.reminder." + strings.TrimSpace(digest.Kind),
			Title:   title,
			Message: message,
		})
	}

	return s.notificationsService.CreateMany(ctx, params)
}

func (s *ReimbursementsService) shouldRunReminder(ctx context.Context, kind string, now time.Time) bool {
	info, ok := tenant.FromContext(ctx)
	if !ok || strings.TrimSpace(info.ID) == "" {
		return true
	}

	runKey := strings.TrimSpace(info.ID) + ":" + strings.TrimSpace(kind)
	runAt := now.In(time.Local).Truncate(time.Minute)

	s.reminderMu.Lock()
	defer s.reminderMu.Unlock()

	if lastRunAt, exists := s.lastReminderRuns[runKey]; exists && lastRunAt.Equal(runAt) {
		return false
	}
	s.lastReminderRuns[runKey] = runAt
	return true
}

func reminderTitle(kind string) string {
	switch strings.TrimSpace(kind) {
	case "payment":
		return "Reminder pembayaran reimbursement"
	default:
		return "Reminder review reimbursement"
	}
}

func inAppReminderMessage(digest model.ReimbursementReminderDigest) (string, string) {
	actionLabel := "menunggu review"
	if strings.TrimSpace(digest.Kind) == "payment" {
		actionLabel = "approved dan menunggu pembayaran"
	}

	message := fmt.Sprintf(
		"Ada %d reimbursement %s dengan total %s.",
		digest.PendingCount,
		actionLabel,
		formatReminderAmount(digest.TotalAmount),
	)
	if digest.OldestCreatedAt != nil {
		message += " Item terlama sejak " + digest.OldestCreatedAt.In(time.Local).Format("02 Jan 2006") + "."
	}

	return reminderTitle(digest.Kind), message
}

func intersectUserIDs(left []string, right []string) []string {
	rightSet := make(map[string]struct{}, len(right))
	for _, userID := range right {
		trimmed := strings.TrimSpace(userID)
		if trimmed == "" {
			continue
		}
		rightSet[trimmed] = struct{}{}
	}

	items := make([]string, 0, len(left))
	seen := make(map[string]struct{}, len(left))
	for _, userID := range left {
		trimmed := strings.TrimSpace(userID)
		if trimmed == "" {
			continue
		}
		if _, ok := rightSet[trimmed]; !ok {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		items = append(items, trimmed)
	}
	return items
}

func formatReminderAmount(amount int64) string {
	value := strconv.FormatInt(amount, 10)
	if len(value) <= 3 {
		return "Rp " + value
	}
	var result []byte
	for index, char := range value {
		if index > 0 && (len(value)-index)%3 == 0 {
			result = append(result, '.')
		}
		result = append(result, byte(char))
	}
	return "Rp " + string(result)
}

func reminderCronMatches(expression string, ts time.Time) bool {
	fields := strings.Fields(strings.TrimSpace(expression))
	if len(fields) != 5 {
		return false
	}

	ts = ts.In(time.Local)
	return matchReminderCronField(fields[0], ts.Minute(), 0, 59, false) &&
		matchReminderCronField(fields[1], ts.Hour(), 0, 23, false) &&
		matchReminderCronField(fields[2], ts.Day(), 1, 31, false) &&
		matchReminderCronField(fields[3], int(ts.Month()), 1, 12, false) &&
		matchReminderCronField(fields[4], int(ts.Weekday()), 0, 6, true)
}

func matchReminderCronField(field string, value int, min int, max int, weekday bool) bool {
	for _, part := range strings.Split(field, ",") {
		if reminderCronPartMatches(strings.TrimSpace(part), value, min, max, weekday) {
			return true
		}
	}
	return false
}

func reminderCronPartMatches(part string, value int, min int, max int, weekday bool) bool {
	if part == "" {
		return false
	}

	step := 1
	base := part
	if strings.Contains(part, "/") {
		pieces := strings.SplitN(part, "/", 2)
		if len(pieces) != 2 {
			return false
		}
		base = pieces[0]
		parsedStep, err := strconv.Atoi(strings.TrimSpace(pieces[1]))
		if err != nil || parsedStep <= 0 {
			return false
		}
		step = parsedStep
	}

	start := min
	end := max
	switch {
	case base == "" || base == "*":
	case strings.Contains(base, "-"):
		rangeParts := strings.SplitN(base, "-", 2)
		if len(rangeParts) != 2 {
			return false
		}
		parsedStart, err := parseReminderCronNumber(rangeParts[0], min, max, weekday)
		if err != nil {
			return false
		}
		parsedEnd, err := parseReminderCronNumber(rangeParts[1], min, max, weekday)
		if err != nil {
			return false
		}
		start = parsedStart
		end = parsedEnd
	default:
		single, err := parseReminderCronNumber(base, min, max, weekday)
		if err != nil {
			return false
		}
		start = single
		end = single
	}

	if start > end {
		return false
	}
	for candidate := start; candidate <= end; candidate += step {
		if normalizeReminderCronValue(candidate, weekday) == value {
			return true
		}
	}
	return false
}

func parseReminderCronNumber(value string, min int, max int, weekday bool) (int, error) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, err
	}
	parsed = normalizeReminderCronValue(parsed, weekday)
	if parsed < min || parsed > max {
		return 0, strconv.ErrSyntax
	}
	return parsed, nil
}

func normalizeReminderCronValue(value int, weekday bool) int {
	if weekday && value == 7 {
		return 0
	}
	return value
}
