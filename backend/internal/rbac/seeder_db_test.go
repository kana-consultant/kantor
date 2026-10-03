package rbac

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

// TestSeedDefaultsUpgradeOnlyAdds runs SeedDefaults against a migrated
// database (KANTOR_TEST_DATABASE_URL) inside one rolled-back transaction,
// on a tenant put back to the previous release's state (baseline v1, no HR
// document grants, a revoked Admin grant, a custom role) and customised by
// its admins (description and hierarchy level of two system roles, a system
// role emptied of every grant). The upgrade start may only ADD the v2 grants
// to Admin: no grant is removed, the revoked one stays revoked, the emptied
// role stays empty, the custom role and every role row (customised values
// and updated_at included) are untouched, and no existing setting row
// changes (the legacy baseline marker included): v2 is recorded as a new
// 'rbac_baseline_v2' row.
func TestSeedDefaultsUpgradeOnlyAdds(t *testing.T) {
	dsn := os.Getenv("KANTOR_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("KANTOR_TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	var tenantID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM tenants ORDER BY created_at LIMIT 1`).Scan(&tenantID); err != nil {
		t.Fatalf("find a tenant: %v", err)
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT set_config('app.current_tenant', $1, false)`, tenantID); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = conn.Exec(context.Background(), "RESET ALL") }()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	ctx = repository.WithConn(ctx, tx)

	// Bring the tenant to the current definitions first (no-op on an
	// up-to-date database), then rewind it to the previous release.
	if err := SeedDefaults(ctx, pool); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	var adminID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM roles WHERE slug = $1`, RoleAdmin).Scan(&adminID); err != nil {
		t.Fatal(err)
	}
	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	v2 := baselinePermissionVersions[2]
	mustExec(`DELETE FROM role_permissions WHERE permission_id = ANY($1::text[])`, v2)
	mustExec(`UPDATE system_settings SET value = '{"version": 1}'::jsonb WHERE key = 'rbac_baseline_version'`)
	mustExec(`DELETE FROM system_settings WHERE key ~ '^rbac_baseline_v[0-9]+$'`)
	// An Admin grant revoked by the tenant on purpose.
	revoked := "hris:salary_safety:view"
	mustExec(`DELETE FROM role_permissions WHERE role_id = $1::uuid AND permission_id = $2`, adminID, revoked)
	// A custom role with its own grants.
	var customID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO roles (name, slug, description, is_system, hierarchy_level)
		VALUES ('Seeder Test HR', 'seeder-test-hr', 'custom', FALSE, 40)
		RETURNING id::text`).Scan(&customID); err != nil {
		t.Fatal(err)
	}
	mustExec(`INSERT INTO role_permissions (role_id, permission_id) VALUES ($1::uuid, 'hris:employee:view')`, customID)
	// System-role metadata an admin may change (PUT /admin/roles/{id} locks
	// only name and slug), and a system role emptied on purpose.
	mustExec(`UPDATE roles SET description = 'Dikustom oleh admin', hierarchy_level = hierarchy_level + 5
		WHERE tenant_id = $1::uuid AND slug IN ($2, $3)`, tenantID, RoleAdmin, RoleViewer)
	var viewerID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM roles WHERE tenant_id = $1::uuid AND slug = $2`, tenantID, RoleViewer).Scan(&viewerID); err != nil {
		t.Fatal(err)
	}
	mustExec(`DELETE FROM role_permissions WHERE role_id = $1::uuid`, viewerID)
	// Fixed timestamps make any rewrite of a role row visible (NOW() is
	// constant inside the transaction).
	mustExec(`UPDATE roles SET updated_at = TIMESTAMPTZ '2001-02-03 04:05:06+00'`)

	type snapshot struct {
		grants   []string
		roles    string
		settings map[string]string
	}
	take := func() snapshot {
		t.Helper()
		var s snapshot
		rows, err := tx.Query(ctx, `SELECT role_id::text || ' ' || permission_id FROM role_permissions ORDER BY 1`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var grant string
			if err := rows.Scan(&grant); err != nil {
				t.Fatal(err)
			}
			s.grants = append(s.grants, grant)
		}
		rows.Close()
		if err := tx.QueryRow(ctx, `SELECT COALESCE(md5(string_agg(r::text, '|' ORDER BY r.id)), '') FROM roles r`).Scan(&s.roles); err != nil {
			t.Fatal(err)
		}
		s.settings = map[string]string{}
		rows, err = tx.Query(ctx, `SELECT key, value::text || ' ' || updated_at::text FROM system_settings`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var key, value string
			if err := rows.Scan(&key, &value); err != nil {
				t.Fatal(err)
			}
			s.settings[key] = value
		}
		rows.Close()
		return s
	}

	before := take()
	if err := SeedDefaults(ctx, pool); err != nil {
		t.Fatalf("upgrade seed: %v", err)
	}
	after := take()

	for _, grant := range before.grants {
		if !slices.Contains(after.grants, grant) {
			t.Errorf("grant removed: %s", grant)
		}
	}
	var added []string
	for _, grant := range after.grants {
		if !slices.Contains(before.grants, grant) {
			added = append(added, grant)
		}
	}
	var wantAdded []string
	for _, permissionID := range v2 {
		wantAdded = append(wantAdded, adminID+" "+permissionID)
	}
	slices.Sort(added)
	slices.Sort(wantAdded)
	if !slices.Equal(added, wantAdded) {
		t.Errorf("added grants = %v, want exactly Admin x v2 = %v", added, wantAdded)
	}
	if slices.Contains(after.grants, adminID+" "+revoked) {
		t.Errorf("revoked Admin grant %s was granted again", revoked)
	}
	for _, grant := range after.grants {
		if strings.HasPrefix(grant, viewerID+" ") {
			t.Errorf("emptied Viewer role got a grant back: %s", grant)
		}
	}
	if after.roles != before.roles {
		t.Errorf("role rows changed by the seed")
	}
	for key, value := range before.settings {
		if after.settings[key] != value {
			t.Errorf("existing setting %s changed: %q -> %q", key, value, after.settings[key])
		}
	}
	for key := range after.settings {
		if _, existed := before.settings[key]; !existed && key != "rbac_baseline_v2" {
			t.Errorf("unexpected new setting %s", key)
		}
	}
	if got := after.settings["rbac_baseline_v2"]; got == "" || !strings.Contains(got, `"version": 2`) {
		t.Errorf("baseline v2 marker not recorded: %q", got)
	}

	// A second start changes nothing at all.
	if err := SeedDefaults(ctx, pool); err != nil {
		t.Fatalf("second seed: %v", err)
	}
	again := take()
	if !slices.Equal(again.grants, after.grants) || again.roles != after.roles {
		t.Errorf("second start changed grants or roles")
	}
	for key, value := range after.settings {
		if again.settings[key] != value {
			t.Errorf("second start changed setting %s", key)
		}
	}
	if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
