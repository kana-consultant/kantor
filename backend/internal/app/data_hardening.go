package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kana-consultant/kantor/backend/internal/config"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	auditrepo "github.com/kana-consultant/kantor/backend/internal/repository/audit"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
	"github.com/kana-consultant/kantor/backend/internal/tenant"
)

// runDataHardening runs the idempotent per-tenant data step that needs the
// application key, after the SQL migrations: bank account numbers are
// encrypted into employees.bank_account_encrypted. By default it is
// additive: the plaintext column, updated_at and every other existing value
// stay as they are. Only the opt-in BANK_ACCOUNT_CLEAR_PLAINTEXT
// (maintenance.BankAccountClearPlaintext, already applied to the repository)
// also clears the plaintext column.
//
// Failures are logged and do not stop the server: reads use the plaintext
// column for rows not yet encrypted, and the step runs again on the next
// start. Logs carry counts only, never values.
func runDataHardening(ctx context.Context, pool *pgxpool.Pool, employees *hrisrepo.EmployeesRepository, maintenance config.DataMaintenanceConfig) {
	logDataMaintenance(ctx, maintenance)
	err := platformmiddleware.ForEachTenant(ctx, pool, func(tCtx context.Context, t tenant.Info) error {
		backfill, err := employees.BackfillBankAccountEncryption(tCtx)
		if err != nil {
			slog.ErrorContext(tCtx, "bank account encryption backfill failed",
				"tenant", t.Slug,
				"error", err,
				"encrypted", backfill.Encrypted,
				"cleared", backfill.Cleared,
			)
		} else if backfill.Candidates > 0 || backfill.Remaining > 0 || backfill.Unsealed > 0 {
			slog.InfoContext(tCtx, "bank account encryption backfill",
				"tenant", t.Slug,
				"plaintext_kept", !backfill.ClearPlaintext,
				"candidates", backfill.Candidates,
				"in_sync", backfill.InSync,
				"encrypted", backfill.Encrypted,
				"superseded", backfill.Superseded,
				"cleared", backfill.Cleared,
				"skipped", backfill.Skipped,
				"unsealed", backfill.Unsealed,
				"remaining_plaintext", backfill.Remaining,
			)
		}
		return nil
	})
	if err != nil {
		slog.ErrorContext(ctx, "data hardening: iterate tenants", "error", err)
	}
}

// logDataMaintenance writes one startup line per opt-in clean-up saying
// whether it is on, and how to turn it on after a backup when it is off.
func logDataMaintenance(ctx context.Context, maintenance config.DataMaintenanceConfig) {
	for _, key := range maintenance.Invalid {
		slog.WarnContext(ctx, "invalid boolean for an opt-in data clean-up; treated as off", "variable", key)
	}
	if maintenance.BankAccountClearPlaintext {
		slog.WarnContext(ctx, "BANK_ACCOUNT_CLEAR_PLAINTEXT=true: bank account numbers are stored encrypted only and the plaintext column employees.bank_account_number is cleared (irreversible)")
	} else {
		slog.InfoContext(ctx, "bank account numbers: plaintext column kept next to the encrypted copy (dual-write); to clear it, take a verified backup, then set BANK_ACCOUNT_CLEAR_PLAINTEXT=true and restart")
	}
	if maintenance.AuditScrubExisting {
		slog.WarnContext(ctx, "AUDIT_SCRUB_EXISTING=true: existing audit_logs rows are redacted in the background (irreversible)")
	} else {
		slog.InfoContext(ctx, "audit log scrub of existing rows is off (new rows are redacted on insert); to redact existing rows, take a verified backup, then set AUDIT_SCRUB_EXISTING=true and restart")
	}
}

// auditScrubJob returns the background job that redacts the existing
// audit_logs rows, or nil when the AUDIT_SCRUB_EXISTING opt-in is off (the
// default) or there is no audit repository.
func auditScrubJob(ctx context.Context, enabled bool, pool *pgxpool.Pool, audit *auditrepo.Repository) func() {
	if !enabled || audit == nil {
		return nil
	}
	return func() {
		runAuditRedactionScrub(ctx, pool, audit)
	}
}

// runAuditRedactionScrub applies the audit denylist to the existing
// audit_logs rows of every tenant (see auditrepo.ScrubRedactedKeys). It runs
// only with the AUDIT_SCRUB_EXISTING opt-in (see auditScrubJob), as a
// background job after startup because a large audit table takes a while;
// new rows are always redacted on insert. The scrub saves its
// position as it goes, so a failed or interrupted run resumes on the next
// start. Logs carry counts only, never values.
func runAuditRedactionScrub(ctx context.Context, pool *pgxpool.Pool, audit *auditrepo.Repository) {
	err := platformmiddleware.ForEachTenant(ctx, pool, func(tCtx context.Context, t tenant.Info) error {
		started := time.Now()
		scrub, err := audit.ScrubRedactedKeys(tCtx)
		if err != nil && ctx.Err() != nil {
			// Shutdown: the position is saved, the next start resumes.
			slog.InfoContext(context.WithoutCancel(tCtx), "audit log redaction scrub interrupted; resumes on next start",
				"tenant", t.Slug,
				"scanned", scrub.Scanned,
				"updated", scrub.Updated,
			)
		} else if err != nil {
			slog.ErrorContext(tCtx, "audit log redaction scrub failed",
				"tenant", t.Slug,
				"error", err,
				"scanned", scrub.Scanned,
				"updated", scrub.Updated,
			)
		} else if !scrub.AlreadyDone {
			slog.InfoContext(tCtx, "audit log redaction scrub",
				"tenant", t.Slug,
				"resumed", scrub.Resumed,
				"scanned", scrub.Scanned,
				"updated", scrub.Updated,
				"duration_ms", time.Since(started).Milliseconds(),
			)
		}
		// One tenant's failure must not keep the others unscrubbed.
		return nil
	})
	if err != nil && ctx.Err() == nil {
		slog.ErrorContext(ctx, "audit log redaction scrub: iterate tenants", "error", err)
	}
}
