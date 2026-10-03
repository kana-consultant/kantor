package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

// Rows written before the denylist existed may still hold account numbers,
// salaries or secrets. ScrubRedactedKeys rewrites them once per tenant; the
// system_settings marker below records that the scrub ran for the current
// denylist. Bump auditRedactionVersion whenever redactedKeys (or
// scopedRedactions) grows so existing rows are scrubbed again for the new
// keys.
//
// Version history:
//
//	1: the redactedKeys denylist.
//	2: also the live registration code that registration-settings updates
//	   used to audit (see scopedRedactions).
const (
	auditRedactionSettingKey = "audit_redaction_version"
	auditRedactionVersion    = 2
	// auditScrubWindow is how many rows (in primary-key order) one batch
	// reads. Each batch is a bounded index range scan, so its cost does not
	// grow with the size of the table.
	auditScrubWindow = 1000
	// auditScrubQueryTimeout is the budget of one scrub statement. It is
	// longer than the per-request repository.DefaultQueryTimeout because a
	// window carries up to auditScrubWindow JSON documents.
	auditScrubQueryTimeout = 60 * time.Second
)

// scopedRedaction redacts keys that are only secret on some audit rows (a
// generic key such as "code" is harmless elsewhere, so it is not in the
// global denylist).
type scopedRedaction struct {
	resource   string
	resourceID string
	keys       []string
}

// scopedRedactions: registration-settings updates audited the decrypted
// registration code until the handler started dropping it.
var scopedRedactions = []scopedRedaction{
	{resource: "system_setting", resourceID: "registration", keys: []string{"code"}},
}

// ScrubResult counts what one scrub run did.
type ScrubResult struct {
	// AlreadyDone: the tenant was scrubbed for this denylist version before.
	AlreadyDone bool
	// Resumed: the run continued an interrupted earlier run.
	Resumed bool
	// Scanned: rows whose JSON text mentions a denylisted key.
	Scanned int
	// Updated: rows that had a value replaced (including rows updated by an
	// interrupted earlier run this one resumed).
	Updated int
}

// auditRedactionMarker is the JSON value of the system_settings marker.
// Version is the last denylist version fully applied; while a run is in
// progress, ResumeVersion/ResumeAfterID record how far it got so an
// interrupted run (timeout, restart) continues instead of starting over.
type auditRedactionMarker struct {
	Version       int    `json:"version"`
	ScrubbedRows  int    `json:"scrubbed_rows"`
	ResumeVersion int    `json:"resume_version,omitempty"`
	ResumeAfterID string `json:"resume_after_id,omitempty"`
}

// ScrubRedactedKeys redacts the denylisted keys in the existing audit_logs
// rows of the tenant in ctx (see RedactJSON). It walks the table in
// primary-key windows, saves its position after every window and is safe to
// re-run: rows are only rewritten when a value changes, an interrupted run
// resumes where it stopped, and the run is skipped once the marker says this
// denylist version is done.
func (r *Repository) ScrubRedactedKeys(ctx context.Context) (ScrubResult, error) {
	var result ScrubResult

	marker, err := r.loadAuditRedactionMarker(ctx)
	if err != nil {
		return result, err
	}
	if marker.Version >= auditRedactionVersion {
		result.AlreadyDone = true
		return result, nil
	}

	pattern := redactedKeyPattern()
	lastID := "00000000-0000-0000-0000-000000000000"
	if marker.ResumeVersion == auditRedactionVersion && marker.ResumeAfterID != "" {
		lastID = marker.ResumeAfterID
		result.Resumed = true
		result.Updated = marker.ScrubbedRows
	}

	for {
		window, err := r.listScrubWindow(ctx, lastID, pattern)
		if err != nil {
			return result, err
		}
		if len(window) == 0 {
			break
		}
		for _, row := range window {
			lastID = row.id
			if !row.candidate {
				continue
			}
			result.Scanned++

			changed, err := r.scrubRow(ctx, row)
			if err != nil {
				return result, err
			}
			if changed {
				result.Updated++
			}
		}
		if len(window) < auditScrubWindow {
			break
		}
		progress := auditRedactionMarker{
			Version:       marker.Version,
			ScrubbedRows:  result.Updated,
			ResumeVersion: auditRedactionVersion,
			ResumeAfterID: lastID,
		}
		if err := r.storeAuditRedactionMarker(ctx, progress); err != nil {
			return result, err
		}
	}

	done := auditRedactionMarker{Version: auditRedactionVersion, ScrubbedRows: result.Updated}
	if err := r.storeAuditRedactionMarker(ctx, done); err != nil {
		return result, err
	}
	return result, nil
}

type scrubCandidate struct {
	id         string
	resource   string
	resourceID string
	// candidate: the row may hold something to redact; only candidates
	// carry their values.
	candidate bool
	oldValue  []byte
	newValue  []byte
}

func (r *Repository) scrubRow(ctx context.Context, row scrubCandidate) (bool, error) {
	extraKeys := scopedRedactionKeys(row.resource, row.resourceID)

	oldValue, oldChanged, err := redactJSONWith(row.oldValue, extraKeys)
	if err != nil {
		return false, fmt.Errorf("redact old_value of audit row %s: %w", row.id, err)
	}
	newValue, newChanged, err := redactJSONWith(row.newValue, extraKeys)
	if err != nil {
		return false, fmt.Errorf("redact new_value of audit row %s: %w", row.id, err)
	}
	if !oldChanged && !newChanged {
		return false, nil
	}
	if err := r.rewriteAuditValues(ctx, row.id, oldValue, newValue); err != nil {
		return false, fmt.Errorf("scrub audit row %s: %w", row.id, err)
	}
	return true, nil
}

// listScrubWindow returns the next auditScrubWindow rows after afterID in
// primary-key order. The ORDER BY / LIMIT run on the uuid column, so the
// primary key index drives the keyset; the regex is evaluated only inside
// the window and only narrows what is sent back (it matches a superset of
// the denylisted keys; RedactJSON decides what actually changes).
func (r *Repository) listScrubWindow(ctx context.Context, afterID string, pattern string) ([]scrubCandidate, error) {
	ctx, cancel := context.WithTimeout(ctx, auditScrubQueryTimeout)
	defer cancel()

	scopedResources, scopedIDs := scopedRedactionColumns()
	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		WITH audit_window AS (
			SELECT id, resource, resource_id, old_value, new_value
			FROM audit_logs
			WHERE id > $1::uuid
			ORDER BY id
			LIMIT $3
		), checked AS (
			SELECT
				id,
				resource,
				resource_id,
				old_value,
				new_value,
				(
					COALESCE(old_value::text ~* $2, false)
					OR COALESCE(new_value::text ~* $2, false)
					OR (resource, resource_id) IN (
						SELECT * FROM unnest($4::text[], $5::text[])
					)
				) AS candidate
			FROM audit_window
		)
		SELECT
			id::text,
			resource,
			COALESCE(resource_id, ''),
			candidate,
			CASE WHEN candidate THEN old_value END,
			CASE WHEN candidate THEN new_value END
		FROM checked
		ORDER BY checked.id
	`, afterID, pattern, auditScrubWindow, scopedResources, scopedIDs)
	if err != nil {
		return nil, fmt.Errorf("list audit rows to scrub: %w", err)
	}
	defer rows.Close()

	items := make([]scrubCandidate, 0, auditScrubWindow)
	for rows.Next() {
		var item scrubCandidate
		if err := rows.Scan(&item.id, &item.resource, &item.resourceID, &item.candidate, &item.oldValue, &item.newValue); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) rewriteAuditValues(ctx context.Context, id string, oldValue []byte, newValue []byte) error {
	ctx, cancel := context.WithTimeout(ctx, auditScrubQueryTimeout)
	defer cancel()

	_, err := repository.DB(ctx, r.db).Exec(ctx, `
		UPDATE audit_logs SET old_value = $2, new_value = $3 WHERE id = $1::uuid
	`, id, nullableJSON(oldValue), nullableJSON(newValue))
	return err
}

func (r *Repository) loadAuditRedactionMarker(ctx context.Context) (auditRedactionMarker, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var raw []byte
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT value FROM system_settings WHERE key = $1
	`, auditRedactionSettingKey).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return auditRedactionMarker{}, nil
	}
	if err != nil {
		return auditRedactionMarker{}, fmt.Errorf("read audit redaction marker: %w", err)
	}
	var marker auditRedactionMarker
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &marker); err != nil {
			// An unreadable marker only costs a full re-scan.
			return auditRedactionMarker{}, nil
		}
	}
	return marker, nil
}

func (r *Repository) storeAuditRedactionMarker(ctx context.Context, marker auditRedactionMarker) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	value, err := json.Marshal(struct {
		auditRedactionMarker
		ScrubbedAt time.Time `json:"scrubbed_at"`
	}{marker, time.Now().UTC()})
	if err != nil {
		return err
	}
	_, err = repository.DB(ctx, r.db).Exec(ctx, `
		INSERT INTO system_settings (key, value, description, updated_at)
		VALUES ($1, $2::jsonb, 'Audit log redaction scrub marker (internal)', NOW())
		ON CONFLICT (tenant_id, key) DO UPDATE
		SET value = EXCLUDED.value, updated_at = NOW()
	`, auditRedactionSettingKey, value)
	if err != nil {
		return fmt.Errorf("store audit redaction marker: %w", err)
	}
	return nil
}

// scopedRedactionKeys returns the extra keys to redact on a row of the
// given resource.
func scopedRedactionKeys(resource string, resourceID string) []string {
	var keys []string
	for _, scoped := range scopedRedactions {
		if scoped.resource == resource && scoped.resourceID == resourceID {
			keys = append(keys, scoped.keys...)
		}
	}
	return keys
}

func scopedRedactionColumns() ([]string, []string) {
	resources := make([]string, 0, len(scopedRedactions))
	ids := make([]string, 0, len(scopedRedactions))
	for _, scoped := range scopedRedactions {
		resources = append(resources, scoped.resource)
		ids = append(ids, scoped.resourceID)
	}
	return resources, ids
}

// redactedKeyPattern is a case-insensitive POSIX regex matching a JSON key
// of the denylist, with optional '_', '-' or ' ' between its letters (the
// separators normalizeAuditKey drops).
func redactedKeyPattern() string {
	keys := make([]string, 0, len(redactedKeys))
	for key := range redactedKeys {
		letters := make([]string, 0, len(key))
		for _, r := range key {
			letters = append(letters, string(r))
		}
		keys = append(keys, strings.Join(letters, "[-_ ]?"))
	}
	sort.Strings(keys)
	return `"(` + strings.Join(keys, "|") + `)"[[:space:]]*:`
}

func nullableJSON(value []byte) any {
	if len(value) == 0 {
		return nil
	}
	return value
}
