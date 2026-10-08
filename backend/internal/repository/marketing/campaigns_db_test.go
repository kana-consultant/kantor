package marketing

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	marketingdto "github.com/kana-consultant/kantor/backend/internal/dto/marketing"
	"github.com/kana-consultant/kantor/backend/internal/model"
	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

// The tests in this file need a migrated database (KANTOR_TEST_DATABASE_URL)
// and are skipped without one. Each test runs in one transaction that is
// rolled back, on two throw-away tenants of its own, under the kantor_app
// role, so row-level security separates the tenants exactly as it does for a
// request (the login role of a dev database is a superuser and would see
// every tenant's rows).

type campaignTestEnv struct {
	t       *testing.T
	ctx     context.Context
	tx      pgx.Tx
	repo    *CampaignsRepository
	metrics *AdsMetricsRepository
	// tenants[0] is the current tenant after setup.
	tenants []campaignTestTenant
}

type campaignTestTenant struct {
	id     string
	userID string
}

func newCampaignTestEnv(t *testing.T) *campaignTestEnv {
	t.Helper()
	dsn := os.Getenv("KANTOR_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("KANTOR_TEST_DATABASE_URL not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	t.Cleanup(conn.Release)
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })

	env := &campaignTestEnv{
		t:       t,
		ctx:     repository.WithConn(ctx, tx),
		tx:      tx,
		repo:    NewCampaignsRepository(pool),
		metrics: NewAdsMetricsRepository(pool),
	}

	// Fixtures are written by the login role, before row-level security
	// applies to this transaction.
	for index := 0; index < 2; index++ {
		suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
		var tenant campaignTestTenant
		env.mustScan(&tenant.id, `INSERT INTO tenants (name, slug) VALUES ($1, $2) RETURNING id::text`,
			"Fixture Tenant "+suffix, "fixture-"+suffix)
		env.mustScan(&tenant.userID, `
			INSERT INTO users (tenant_id, email, password_hash, full_name)
			VALUES ($1::uuid, $2, 'not-a-hash', 'Fixture User')
			RETURNING id::text
		`, tenant.id, "fixture-"+suffix+"@example.test")
		env.tenants = append(env.tenants, tenant)
	}

	env.mustExec(`SET LOCAL ROLE kantor_app`)
	env.useTenant(0)
	return env
}

func (e *campaignTestEnv) useTenant(index int) {
	e.t.Helper()
	e.mustExec(`SELECT set_config('app.current_tenant', $1, true)`, e.tenants[index].id)
}

func (e *campaignTestEnv) mustExec(sql string, args ...any) {
	e.t.Helper()
	if _, err := e.tx.Exec(e.ctx, sql, args...); err != nil {
		e.t.Fatalf("%s: %v", sql, err)
	}
}

func (e *campaignTestEnv) mustScan(dest any, sql string, args ...any) {
	e.t.Helper()
	if err := e.tx.QueryRow(e.ctx, sql, args...).Scan(dest); err != nil {
		e.t.Fatalf("%s: %v", sql, err)
	}
}

// boardRow is everything about a column that the stage guarantee must leave
// alone, plus the stage it may set.
type boardRow struct {
	ID       string
	Name     string
	Position int
	Color    string
	Stage    string
}

func (e *campaignTestEnv) board() []boardRow {
	e.t.Helper()
	rows, err := e.tx.Query(e.ctx, `
		SELECT id::text, name, position, COALESCE(color, ''), COALESCE(stage, '')
		FROM campaign_columns
		ORDER BY position ASC
	`)
	if err != nil {
		e.t.Fatalf("read board: %v", err)
	}
	defer rows.Close()
	var items []boardRow
	for rows.Next() {
		var row boardRow
		if err := rows.Scan(&row.ID, &row.Name, &row.Position, &row.Color, &row.Stage); err != nil {
			e.t.Fatalf("scan board: %v", err)
		}
		items = append(items, row)
	}
	if err := rows.Err(); err != nil {
		e.t.Fatalf("read board: %v", err)
	}
	return items
}

// seedColumn inserts a column the way an older release or an admin would
// have left it. stage "" means no stage.
func (e *campaignTestEnv) seedColumn(name string, position int, color string, stage string) string {
	e.t.Helper()
	var id string
	e.mustScan(&id, `
		INSERT INTO campaign_columns (name, position, color, stage)
		VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''))
		RETURNING id::text
	`, name, position, color, stage)
	return id
}

func (e *campaignTestEnv) campaignParams(name string, status string) UpsertCampaignParams {
	return UpsertCampaignParams{
		Name:           name,
		Channel:        "other",
		BudgetAmount:   700000,
		BudgetCurrency: "IDR",
		StartDate:      time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC),
		EndDate:        time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC),
		Status:         status,
		ActorID:        e.tenants[0].userID,
	}
}

func (e *campaignTestEnv) createCampaign(name string, status string) model.Campaign {
	e.t.Helper()
	item, err := e.repo.CreateCampaign(e.ctx, e.campaignParams(name, status))
	if err != nil {
		e.t.Fatalf("create campaign %q (%s): %v", name, status, err)
	}
	return item
}

func (e *campaignTestEnv) ensure() EnsureStageColumnsResult {
	e.t.Helper()
	result, err := e.repo.EnsureStageColumns(e.ctx)
	if err != nil {
		e.t.Fatalf("EnsureStageColumns: %v", err)
	}
	return result
}

func stagesOf(board []boardRow) map[string]boardRow {
	byStage := map[string]boardRow{}
	for _, row := range board {
		if row.Stage != "" {
			byStage[row.Stage] = row
		}
	}
	return byStage
}

func deref[T any](value *T) T {
	var zero T
	if value == nil {
		return zero
	}
	return *value
}

// assertOnlyStageGained checks the owner's condition for existing boards:
// every column that existed before is still there with the same id, name,
// position and colour; the only thing that may have changed is an empty
// stage that is now set.
func assertOnlyStageGained(t *testing.T, before []boardRow, after []boardRow) {
	t.Helper()
	afterByID := map[string]boardRow{}
	for _, row := range after {
		afterByID[row.ID] = row
	}
	for _, old := range before {
		now, ok := afterByID[old.ID]
		if !ok {
			t.Errorf("column %q (%s) was deleted", old.Name, old.ID)
			continue
		}
		if now.Name != old.Name || now.Position != old.Position || now.Color != old.Color {
			t.Errorf("column %s changed: before %+v, after %+v", old.ID, old, now)
		}
		if old.Stage != "" && now.Stage != old.Stage {
			t.Errorf("column %q had stage %q, now %q", old.Name, old.Stage, now.Stage)
		}
	}
}

func assertAllStagesPresent(t *testing.T, board []boardRow) {
	t.Helper()
	byStage := stagesOf(board)
	for _, stage := range marketingdto.CampaignStages {
		if _, ok := byStage[stage]; !ok {
			t.Errorf("stage %q has no column; board: %+v", stage, board)
		}
	}
	seen := map[int]bool{}
	for _, row := range board {
		if seen[row.Position] {
			t.Errorf("position %d is used twice: %+v", row.Position, board)
		}
		seen[row.Position] = true
	}
}

// The stage list of the repository (defaults to append) and the list the
// request validators accept must be the same, in the same order.
func TestDefaultStageColumnsMatchTheValidatedStages(t *testing.T) {
	var stages []string
	for _, def := range defaultStageColumns {
		stages = append(stages, def.Stage)
		if canonicalCampaignState(def.Name) != def.Stage {
			t.Errorf("default column %q does not canonicalise to its stage %q", def.Name, def.Stage)
		}
	}
	if !reflect.DeepEqual(stages, marketingdto.CampaignStages) {
		t.Fatalf("defaultStageColumns = %v, dto CampaignStages = %v", stages, marketingdto.CampaignStages)
	}
}

func TestUnusedColumnName(t *testing.T) {
	used := map[string]bool{"Live": true, "Live (2)": true}
	if got := unusedColumnName("Planning", used); got != "Planning" {
		t.Errorf("free name: got %q", got)
	}
	if got := unusedColumnName("Live", used); got != "Live (3)" {
		t.Errorf("taken name: got %q, want Live (3)", got)
	}
}

// A tenant that never got the default columns (every tenant but the first
// before this fix): the six stage columns are created, a second run changes
// nothing, and another tenant is neither read nor written.
func TestEnsureStageColumnsOnTenantWithoutColumns(t *testing.T) {
	env := newCampaignTestEnv(t)

	if board := env.board(); len(board) != 0 {
		t.Fatalf("fixture tenant should start without columns, has %+v", board)
	}

	result := env.ensure()
	if !reflect.DeepEqual(result.Created, marketingdto.CampaignStages) || len(result.Adopted) != 0 {
		t.Fatalf("first run: created %v adopted %v, want all six created", result.Created, result.Adopted)
	}
	board := env.board()
	if len(board) != 6 {
		t.Fatalf("want 6 columns, got %+v", board)
	}
	for index, def := range defaultStageColumns {
		row := board[index]
		if row.Name != def.Name || row.Color != def.Color || row.Stage != def.Stage || row.Position != index+1 {
			t.Errorf("column %d = %+v, want %+v at position %d", index, row, def, index+1)
		}
		if _, err := uuid.Parse(row.ID); err != nil {
			t.Errorf("column id %q is not a uuid", row.ID)
		}
	}

	// Idempotent.
	again := env.ensure()
	if again.Changed() {
		t.Errorf("second run changed the board: %+v", again)
	}
	if after := env.board(); !reflect.DeepEqual(after, board) {
		t.Errorf("second run rewrote the board:\nbefore %+v\nafter  %+v", board, after)
	}

	// The second tenant was not touched and gets its own six columns.
	env.useTenant(1)
	if other := env.board(); len(other) != 0 {
		t.Fatalf("second tenant got columns from the first tenant's run: %+v", other)
	}
	if result := env.ensure(); len(result.Created) != 6 {
		t.Fatalf("second tenant: created %v", result.Created)
	}
	otherBoard := env.board()
	assertAllStagesPresent(t, otherBoard)
	for _, row := range otherBoard {
		for _, first := range board {
			if row.ID == first.ID {
				t.Errorf("tenants share column %s", row.ID)
			}
		}
	}

	env.useTenant(0)
	if after := env.board(); !reflect.DeepEqual(after, board) {
		t.Errorf("first tenant changed by the second tenant's run:\nbefore %+v\nafter  %+v", board, after)
	}
}

// Boards as they can exist in production before the first boot with this
// release: every column has stage NULL, and admins may have renamed, deleted,
// duplicated or reordered columns through the API. Whatever the board looks
// like, the run must succeed, leave every existing column as it was (apart
// from setting a stage), end with one column per stage, and change nothing
// when run again.
func TestEnsureStageColumnsOnExistingBoards(t *testing.T) {
	type seed struct {
		name     string
		position int
		color    string
		stage    string
	}
	legacy := []seed{
		{"Ideation", 1, "#8B5CF6", ""}, {"Planning", 2, "#0EA5E9", ""}, {"In Production", 3, "#F59E0B", ""},
		{"Live", 4, "#10B981", ""}, {"Completed", 5, "#334155", ""}, {"Archived", 6, "#94A3B8", ""},
	}
	with := func(base []seed, change func([]seed) []seed) []seed {
		return change(append([]seed(nil), base...))
	}

	cases := []struct {
		name        string
		seeds       []seed
		wantAdopted []string
		wantCreated []string
		check       func(t *testing.T, board []boardRow)
	}{
		{
			name:        "untouched default board is adopted in place",
			seeds:       legacy,
			wantAdopted: marketingdto.CampaignStages,
			check: func(t *testing.T, board []boardRow) {
				if len(board) != 6 {
					t.Errorf("want 6 columns, got %d", len(board))
				}
			},
		},
		{
			name: "renamed live column stays custom and a Live column is appended",
			seeds: with(legacy, func(s []seed) []seed {
				s[3].name = "Running Ads"
				return s
			}),
			wantAdopted: []string{"ideation", "planning", "in_production", "completed", "archived"},
			wantCreated: []string{"live"},
			check: func(t *testing.T, board []boardRow) {
				live := stagesOf(board)["live"]
				if live.Name != "Live" || live.Position != 7 {
					t.Errorf("appended live column = %+v, want Live at position 7", live)
				}
				for _, row := range board {
					if row.Name == "Running Ads" && row.Stage != "" {
						t.Errorf("renamed column must stay custom, got stage %q", row.Stage)
					}
				}
			},
		},
		{
			name: "deleted stage column is appended again",
			seeds: with(legacy, func(s []seed) []seed {
				return s[:5] // Archived deleted
			}),
			wantAdopted: []string{"ideation", "planning", "in_production", "live", "completed"},
			wantCreated: []string{"archived"},
		},
		{
			name: "duplicate canonical names: the first by position is adopted",
			seeds: []seed{
				{"LIVE!", 1, "#111111", ""}, {"Planning", 2, "", ""}, {"live", 3, "#222222", ""}, {"In-Production", 4, "", ""},
			},
			wantAdopted: []string{"planning", "in_production", "live"},
			wantCreated: []string{"ideation", "completed", "archived"},
			check: func(t *testing.T, board []boardRow) {
				if live := stagesOf(board)["live"]; live.Name != "LIVE!" {
					t.Errorf("live stage went to %q, want the first by position (LIVE!)", live.Name)
				}
				for _, row := range board {
					if row.Name == "live" && row.Stage != "" {
						t.Errorf("second column named like live must stay custom, got %q", row.Stage)
					}
				}
			},
		},
		{
			name: "position gaps and custom columns: appended after the last one",
			seeds: []seed{
				{"Brief Masuk", 10, "#123456", ""}, {"Live", 20, "", ""}, {"Review Klien", 500, "", ""},
			},
			wantAdopted: []string{"live"},
			wantCreated: []string{"ideation", "planning", "in_production", "completed", "archived"},
			check: func(t *testing.T, board []boardRow) {
				byStage := stagesOf(board)
				for index, stage := range []string{"ideation", "planning", "in_production", "completed", "archived"} {
					if got := byStage[stage].Position; got != 501+index {
						t.Errorf("stage %s appended at %d, want %d", stage, got, 501+index)
					}
				}
			},
		},
		{
			name: "a column named like a default already stands for another stage",
			seeds: []seed{
				{"Live", 1, "#10B981", "planning"}, {"Planning", 2, "", "ideation"},
			},
			wantCreated: []string{"in_production", "live", "completed", "archived"},
			check: func(t *testing.T, board []boardRow) {
				live := stagesOf(board)["live"]
				if live.Name != "Live (2)" {
					t.Errorf("live column appended as %q, want a free name (Live (2))", live.Name)
				}
				if planning := stagesOf(board)["planning"]; planning.Name != "Live" {
					t.Errorf("existing stage column was re-pointed: %+v", planning)
				}
			},
		},
		{
			name: "partly migrated board: existing stages are kept",
			seeds: []seed{
				{"Tayang", 1, "", "live"}, {"Live", 2, "", ""}, {"Ideation", 3, "", ""},
			},
			wantAdopted: []string{"ideation"},
			wantCreated: []string{"planning", "in_production", "completed", "archived"},
			check: func(t *testing.T, board []boardRow) {
				if live := stagesOf(board)["live"]; live.Name != "Tayang" {
					t.Errorf("live stage moved to %q, want it to stay on Tayang", live.Name)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newCampaignTestEnv(t)
			for _, s := range tc.seeds {
				env.seedColumn(s.name, s.position, s.color, s.stage)
			}
			before := env.board()

			result := env.ensure()
			if !reflect.DeepEqual(result.Adopted, tc.wantAdopted) && (len(result.Adopted) != 0 || len(tc.wantAdopted) != 0) {
				t.Errorf("adopted %v, want %v", result.Adopted, tc.wantAdopted)
			}
			if !reflect.DeepEqual(result.Created, tc.wantCreated) && (len(result.Created) != 0 || len(tc.wantCreated) != 0) {
				t.Errorf("created %v, want %v", result.Created, tc.wantCreated)
			}

			after := env.board()
			assertOnlyStageGained(t, before, after)
			assertAllStagesPresent(t, after)
			if len(after) != len(before)+len(tc.wantCreated) {
				t.Errorf("board grew from %d to %d columns, want +%d", len(before), len(after), len(tc.wantCreated))
			}
			if tc.check != nil {
				tc.check(t, after)
			}

			again := env.ensure()
			if again.Changed() {
				t.Errorf("second run changed the board: %+v", again)
			}
			if rerun := env.board(); !reflect.DeepEqual(rerun, after) {
				t.Errorf("second run rewrote the board:\nbefore %+v\nafter  %+v", after, rerun)
			}
		})
	}
}

// Campaigns and their placement are not the guarantee's business: a campaign
// sitting in a renamed lane stays there with its status.
func TestEnsureStageColumnsLeavesCampaignsAlone(t *testing.T) {
	env := newCampaignTestEnv(t)
	renamed := env.seedColumn("Running Ads", 1, "#10B981", "")
	var campaignID string
	env.mustScan(&campaignID, `
		INSERT INTO campaigns (name, channel, start_date, end_date, status, created_by)
		VALUES ('Old live campaign', 'instagram', DATE '2026-01-01', DATE '2026-01-31', 'live', $1::uuid)
		RETURNING id::text
	`, env.tenants[0].userID)
	env.mustExec(`
		INSERT INTO campaign_column_assignments (campaign_id, column_id, position, moved_by)
		VALUES ($1::uuid, $2::uuid, 1, $3::uuid)
	`, campaignID, renamed, env.tenants[0].userID)
	// Row versions (ctid), not timestamps: NOW() does not advance inside the
	// test's transaction, so moved_at/updated_at could not show a rewrite.
	var assignmentBefore, campaignBefore string
	env.mustScan(&assignmentBefore, `SELECT ctid::text FROM campaign_column_assignments WHERE campaign_id = $1::uuid`, campaignID)
	env.mustScan(&campaignBefore, `SELECT ctid::text FROM campaigns WHERE id = $1::uuid`, campaignID)

	env.ensure()

	item, err := env.repo.GetCampaignByID(env.ctx, campaignID)
	if err != nil {
		t.Fatal(err)
	}
	if deref(item.ColumnID) != renamed || deref(item.ColumnPosition) != 1 || item.Status != "live" {
		t.Errorf("campaign moved or changed: column %s position %d status %s", deref(item.ColumnID), deref(item.ColumnPosition), item.Status)
	}
	var assignmentAfter, campaignAfter string
	env.mustScan(&assignmentAfter, `SELECT ctid::text FROM campaign_column_assignments WHERE campaign_id = $1::uuid`, campaignID)
	env.mustScan(&campaignAfter, `SELECT ctid::text FROM campaigns WHERE id = $1::uuid`, campaignID)
	if assignmentAfter != assignmentBefore || campaignAfter != campaignBefore {
		t.Errorf("campaign rows were rewritten (assignment row %s -> %s, campaign row %s -> %s)", assignmentBefore, assignmentAfter, campaignBefore, campaignAfter)
	}
}

// The report: creating a campaign on a tenant whose board has no column for
// the stage. The create repairs the board in its own transaction; every
// stage works, on both tenants, and the tenants do not see each other.
func TestCreateCampaignForEveryStageOnTenantWithoutColumns(t *testing.T) {
	env := newCampaignTestEnv(t)

	for index, stage := range marketingdto.CampaignStages {
		item := env.createCampaign(fmt.Sprintf("Campaign %d", index), stage)
		if item.Status != stage {
			t.Errorf("stage %s: status = %q", stage, item.Status)
		}
		if item.ColumnID == nil || deref(item.ColumnPosition) != 1 {
			t.Fatalf("stage %s: campaign has no placement: %+v", stage, item)
		}
		if got := stagesOf(env.board())[stage].ID; got != *item.ColumnID {
			t.Errorf("stage %s: campaign is in column %s, the stage column is %s", stage, *item.ColumnID, got)
		}
	}
	board := env.board()
	if len(board) != 6 {
		t.Errorf("want exactly the six stage columns after the creates, got %+v", board)
	}

	// Second tenant: still nothing there, and its first create works too.
	env.useTenant(1)
	if other := env.board(); len(other) != 0 {
		t.Fatalf("second tenant sees columns: %+v", other)
	}
	if _, total, err := env.repo.ListCampaigns(env.ctx, ListCampaignsParams{Page: 1, PerPage: 50}); err != nil || total != 0 {
		t.Fatalf("second tenant sees %d campaigns (err %v)", total, err)
	}
	params := env.campaignParams("PRODUCT LAUNCH | TRAFFIC", "live")
	params.ActorID = env.tenants[1].userID
	item, err := env.repo.CreateCampaign(env.ctx, params)
	if err != nil {
		t.Fatalf("create on second tenant: %v", err)
	}
	if item.Status != "live" || deref(item.ColumnName) != "Live" {
		t.Errorf("second tenant campaign: status %q column %q", item.Status, deref(item.ColumnName))
	}

	kanban, err := env.repo.ListKanban(env.ctx)
	if err != nil {
		t.Fatal(err)
	}
	cards := 0
	for _, column := range kanban {
		if column.Campaigns == nil {
			t.Errorf("kanban column %q has nil campaigns; it must serialise as []", column.Name)
		}
		if column.CampaignsNo != len(column.Campaigns) {
			t.Errorf("column %q: campaign_count %d, %d campaigns", column.Name, column.CampaignsNo, len(column.Campaigns))
		}
		cards += len(column.Campaigns)
	}
	if len(kanban) != 6 || cards != 1 {
		t.Errorf("second tenant kanban: %d columns, %d cards; want 6 and 1", len(kanban), cards)
	}
}

// A renamed stage lane still is that stage's lane: the name is only a label.
func TestRenamedStageColumnStillReceivesItsStage(t *testing.T) {
	env := newCampaignTestEnv(t)
	env.ensure()
	live := stagesOf(env.board())["live"]

	renamed, err := env.repo.UpdateColumn(env.ctx, live.ID, UpdateCampaignColumnParams{Name: "Running Ads"})
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if deref(renamed.Stage) != "live" {
		t.Errorf("rename changed the stage to %q", deref(renamed.Stage))
	}
	if deref(renamed.Color) != live.Color {
		t.Errorf("rename without a colour changed it from %q to %q", live.Color, deref(renamed.Color))
	}

	item := env.createCampaign("After rename", "live")
	if deref(item.ColumnID) != live.ID || deref(item.ColumnName) != "Running Ads" || item.Status != "live" {
		t.Errorf("campaign landed in %q (%s) with status %q", deref(item.ColumnName), deref(item.ColumnID), item.Status)
	}
	if len(env.board()) != 6 {
		t.Errorf("create added columns to a complete board: %+v", env.board())
	}

	// An explicit empty colour clears it; a new colour replaces it.
	empty := ""
	cleared, err := env.repo.UpdateColumn(env.ctx, live.ID, UpdateCampaignColumnParams{Name: "Running Ads", Color: &empty})
	if err != nil || cleared.Color != nil {
		t.Errorf("empty colour should clear it: %v, colour %v", err, cleared.Color)
	}
	blue := "#0866FF"
	recoloured, err := env.repo.UpdateColumn(env.ctx, live.ID, UpdateCampaignColumnParams{Name: "Running Ads", Color: &blue})
	if err != nil || deref(recoloured.Color) != blue || recoloured.CampaignsNo != 1 {
		t.Errorf("recolour: %v, colour %q, count %d", err, deref(recoloured.Color), recoloured.CampaignsNo)
	}
}

// placement returns the card's lane and position plus the physical location
// of its assignment row (ctid). Every test runs in one transaction, where
// NOW() never advances, so moved_at cannot show that the row was rewritten;
// an UPDATE always writes a new row version at a new ctid.
func (e *campaignTestEnv) placement(campaignID string) (string, int, string) {
	e.t.Helper()
	var (
		columnID string
		position int
		version  string
	)
	if err := e.tx.QueryRow(e.ctx, `
		SELECT column_id::text, position, ctid::text FROM campaign_column_assignments WHERE campaign_id = $1::uuid
	`, campaignID).Scan(&columnID, &position, &version); err != nil {
		e.t.Fatalf("placement of %s: %v", campaignID, err)
	}
	return columnID, position, version
}

func TestUpdateCampaignMovesOnlyWhenTheStageChanges(t *testing.T) {
	env := newCampaignTestEnv(t)
	first := env.createCampaign("First", "live")
	second := env.createCampaign("Second", "live")
	third := env.createCampaign("Third", "live")
	stages := stagesOf(env.board())

	// Same stage: the assignment row is not even rewritten.
	columnBefore, positionBefore, versionBefore := env.placement(first.ID)
	params := env.campaignParams("First, renamed", "live")
	params.Channel = "meta_ads"
	updated, previous, err := env.repo.UpdateCampaign(env.ctx, first.ID, params)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if previous != "live" || updated.Name != "First, renamed" || updated.Channel != "meta_ads" {
		t.Errorf("update result: previous %q, %+v", previous, updated)
	}
	columnAfter, positionAfter, versionAfter := env.placement(first.ID)
	if columnAfter != columnBefore || positionAfter != positionBefore || positionAfter != 1 || versionAfter != versionBefore {
		t.Errorf("unchanged stage moved the card: column %s -> %s, position %d -> %d, assignment row %s -> %s",
			columnBefore, columnAfter, positionBefore, positionAfter, versionBefore, versionAfter)
	}

	// Every stage: the card goes to the end of that stage's lane.
	env.createCampaign("Already completed", "completed")
	for _, stage := range []string{"completed", "ideation", "planning", "in_production", "archived", "live"} {
		moved, previous, err := env.repo.UpdateCampaign(env.ctx, second.ID, env.campaignParams("Second", stage))
		if err != nil {
			t.Fatalf("update to %s: %v", stage, err)
		}
		if moved.Status != stage || deref(moved.ColumnID) != stages[stage].ID {
			t.Errorf("update to %s: status %q, column %q", stage, moved.Status, deref(moved.ColumnName))
		}
		if previous == stage {
			t.Errorf("update to %s reported the same previous stage", stage)
		}
		if stage == "completed" && deref(moved.ColumnPosition) != 2 {
			t.Errorf("card should go to the end of the lane (2), got %d", deref(moved.ColumnPosition))
		}
	}

	// The lane it left and came back to has no holes or duplicates.
	var positions []int
	rows, err := env.tx.Query(env.ctx, `SELECT position FROM campaign_column_assignments WHERE column_id = $1::uuid ORDER BY position`, stages["live"].ID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var position int
		if err := rows.Scan(&position); err != nil {
			t.Fatal(err)
		}
		positions = append(positions, position)
	}
	rows.Close()
	if !reflect.DeepEqual(positions, []int{1, 2, 3}) {
		t.Errorf("live lane positions = %v, want [1 2 3]", positions)
	}
	if _, position, _ := env.placement(third.ID); position != 2 {
		t.Errorf("third card should have closed the gap to position 2, is at %d", position)
	}

	// A campaign that sits in no lane (legacy data) is placed by an update.
	env.mustExec(`DELETE FROM campaign_column_assignments WHERE campaign_id = $1::uuid`, third.ID)
	placed, _, err := env.repo.UpdateCampaign(env.ctx, third.ID, env.campaignParams("Third", "live"))
	if err != nil || deref(placed.ColumnID) != stages["live"].ID {
		t.Errorf("unplaced campaign: err %v, column %q", err, deref(placed.ColumnName))
	}

	// Unknown campaign.
	if _, _, err := env.repo.UpdateCampaign(env.ctx, uuid.NewString(), env.campaignParams("Nobody", "live")); !errors.Is(err, ErrCampaignNotFound) {
		t.Errorf("unknown campaign: %v, want ErrCampaignNotFound", err)
	}
}

// Custom lanes (no stage) are parking spots: they keep the status, and an
// edit that keeps the stage does not pull the card out of them.
func TestCustomColumnKeepsTheCampaignStatus(t *testing.T) {
	env := newCampaignTestEnv(t)
	item := env.createCampaign("Parked", "live")
	stages := stagesOf(env.board())

	custom, err := env.repo.CreateColumn(env.ctx, CreateCampaignColumnParams{Name: "Review Klien"})
	if err != nil {
		t.Fatalf("create custom column: %v", err)
	}
	if custom.Stage != nil || custom.Position != 7 {
		t.Errorf("custom column: stage %v position %d, want no stage at 7", custom.Stage, custom.Position)
	}

	moved, err := env.repo.MoveCampaign(env.ctx, item.ID, custom.ID, 1, env.tenants[0].userID)
	if err != nil {
		t.Fatalf("move to custom column: %v", err)
	}
	if moved.Status != "live" || deref(moved.ColumnID) != custom.ID {
		t.Errorf("after move to custom: status %q column %q", moved.Status, deref(moved.ColumnName))
	}

	edited, _, err := env.repo.UpdateCampaign(env.ctx, item.ID, env.campaignParams("Parked, edited", "live"))
	if err != nil {
		t.Fatalf("edit in custom column: %v", err)
	}
	if deref(edited.ColumnID) != custom.ID || edited.Status != "live" {
		t.Errorf("edit pulled the card out of the custom column: column %q status %q", deref(edited.ColumnName), edited.Status)
	}

	// Moving into a stage lane sets the status; changing the stage in the
	// form leaves the custom lane.
	toPlanning, err := env.repo.MoveCampaign(env.ctx, item.ID, stages["planning"].ID, 1, env.tenants[0].userID)
	if err != nil || toPlanning.Status != "planning" {
		t.Errorf("move to planning lane: err %v status %q", err, toPlanning.Status)
	}
	if _, err := env.repo.MoveCampaign(env.ctx, item.ID, custom.ID, 1, env.tenants[0].userID); err != nil {
		t.Fatal(err)
	}
	restaged, previous, err := env.repo.UpdateCampaign(env.ctx, item.ID, env.campaignParams("Parked", "completed"))
	if err != nil || previous != "planning" || restaged.Status != "completed" || deref(restaged.ColumnID) != stages["completed"].ID {
		t.Errorf("stage change from a custom column: err %v previous %q status %q column %q", err, previous, restaged.Status, deref(restaged.ColumnName))
	}

	if _, err := env.repo.MoveCampaign(env.ctx, item.ID, uuid.NewString(), 1, env.tenants[0].userID); !errors.Is(err, ErrCampaignColumnNotFound) {
		t.Errorf("move to an unknown column: %v, want ErrCampaignColumnNotFound", err)
	}
}

// A deleted campaign leaves a gap in its lane (production boards have them).
// A move asks for an index into the lane as the board shows it, so "one past
// the last card" and "far past the end" must both put the card last, and a
// drop between two cards must land between them.
func TestMoveIntoLaneWithPositionGaps(t *testing.T) {
	env := newCampaignTestEnv(t)
	first := env.createCampaign("Gap first", "live")
	second := env.createCampaign("Gap second", "live")
	deleted := env.createCampaign("Gap deleted", "live")
	last := env.createCampaign("Gap last", "live")
	if err := env.repo.DeleteCampaign(env.ctx, deleted.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	live := stagesOf(env.board())["live"].ID

	lane := func() []string {
		t.Helper()
		var ids []string
		rows, err := env.tx.Query(env.ctx, `
			SELECT campaign_column_assignments.campaign_id::text
			FROM campaign_column_assignments
			INNER JOIN campaigns ON campaigns.id = campaign_column_assignments.campaign_id
			WHERE column_id = $1::uuid
			ORDER BY campaign_column_assignments.position, campaigns.created_at
		`, live)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				t.Fatal(err)
			}
			ids = append(ids, id)
		}
		return ids
	}
	if _, position, _ := env.placement(last.ID); position != 4 {
		t.Fatalf("fixture: the last card should sit at 4 after a gap, got %d", position)
	}

	// The board shows three cards; dropping after the third asks for 4.
	moving := env.createCampaign("Gap moving", "planning")
	if _, err := env.repo.MoveCampaign(env.ctx, moving.ID, live, 4, env.tenants[0].userID); err != nil {
		t.Fatalf("move to the end: %v", err)
	}
	if got, want := lane(), []string{first.ID, second.ID, last.ID, moving.ID}; !reflect.DeepEqual(got, want) {
		t.Errorf("drop after the last card: lane %v, want %v", got, want)
	}
	if _, position, _ := env.placement(moving.ID); position != 4 {
		t.Errorf("moved card at position %d, want 4 (lane renumbered 1..n)", position)
	}

	// A position far past the end (a filtered board) also means last.
	other := env.createCampaign("Gap other", "planning")
	if _, err := env.repo.MoveCampaign(env.ctx, other.ID, live, 1000, env.tenants[0].userID); err != nil {
		t.Fatalf("move far past the end: %v", err)
	}
	if got := lane(); got[len(got)-1] != other.ID {
		t.Errorf("position past the end did not put the card last: %v", got)
	}

	// Between the second and the third card shown.
	between := env.createCampaign("Gap between", "planning")
	if _, err := env.repo.MoveCampaign(env.ctx, between.ID, live, 3, env.tenants[0].userID); err != nil {
		t.Fatalf("move between: %v", err)
	}
	if got, want := lane(), []string{first.ID, second.ID, between.ID, last.ID, moving.ID, other.ID}; !reflect.DeepEqual(got, want) {
		t.Errorf("drop between cards: lane %v, want %v", got, want)
	}
}

func (e *campaignTestEnv) boardNames() []string {
	var names []string
	for _, row := range e.board() {
		names = append(names, fmt.Sprintf("%d:%s", row.Position, row.Name))
	}
	return names
}

// Column create in the middle, delete in the middle and reorder used to end
// in a unique violation (500). They must work and keep positions contiguous,
// and the caller errors must be typed and leave the transaction usable.
func TestColumnCreateDeleteReorder(t *testing.T) {
	env := newCampaignTestEnv(t)
	env.ensure()

	second := 2
	review, err := env.repo.CreateColumn(env.ctx, CreateCampaignColumnParams{Name: "Review", Position: &second})
	if err != nil {
		t.Fatalf("create at position 2: %v", err)
	}
	want := []string{"1:Ideation", "2:Review", "3:Planning", "4:In Production", "5:Live", "6:Completed", "7:Archived"}
	if got := env.boardNames(); !reflect.DeepEqual(got, want) {
		t.Errorf("after create at 2:\n got %v\nwant %v", got, want)
	}

	first := 1
	if _, err := env.repo.CreateColumn(env.ctx, CreateCampaignColumnParams{Name: "Inbox", Position: &first}); err != nil {
		t.Fatalf("create at position 1: %v", err)
	}
	far := 99
	last, err := env.repo.CreateColumn(env.ctx, CreateCampaignColumnParams{Name: "Parkir", Position: &far})
	if err != nil || last.Position != 9 {
		t.Fatalf("create beyond the end: err %v position %d, want 9", err, last.Position)
	}
	want = []string{"1:Inbox", "2:Ideation", "3:Review", "4:Planning", "5:In Production", "6:Live", "7:Completed", "8:Archived", "9:Parkir"}
	if got := env.boardNames(); !reflect.DeepEqual(got, want) {
		t.Errorf("after three creates:\n got %v\nwant %v", got, want)
	}

	// Duplicate name: typed error, and the transaction still works.
	if _, err := env.repo.CreateColumn(env.ctx, CreateCampaignColumnParams{Name: "Review"}); !errors.Is(err, ErrCampaignColumnNameTaken) {
		t.Errorf("duplicate name on create: %v, want ErrCampaignColumnNameTaken", err)
	}
	// (The rename is a single statement without a transaction of its own; in
	// this test it needs a savepoint so its failure does not abort the test
	// transaction.)
	renameErr := inSavepoint(env.ctx, env.tx, func(sp pgx.Tx) error {
		_, err := env.repo.UpdateColumn(repository.WithConn(env.ctx, sp), last.ID, UpdateCampaignColumnParams{Name: "Review"})
		return err
	})
	if !errors.Is(renameErr, ErrCampaignColumnNameTaken) {
		t.Errorf("duplicate name on rename: %v, want ErrCampaignColumnNameTaken", renameErr)
	}
	if got := env.boardNames(); !reflect.DeepEqual(got, want) {
		t.Errorf("failed writes changed the board: %v", got)
	}

	// Delete a custom column in the middle.
	if err := env.repo.DeleteColumn(env.ctx, review.ID); err != nil {
		t.Fatalf("delete a middle column: %v", err)
	}
	want = []string{"1:Inbox", "2:Ideation", "3:Planning", "4:In Production", "5:Live", "6:Completed", "7:Archived", "8:Parkir"}
	if got := env.boardNames(); !reflect.DeepEqual(got, want) {
		t.Errorf("after delete:\n got %v\nwant %v", got, want)
	}

	// Stage columns are protected; columns holding campaigns are in use.
	stages := stagesOf(env.board())
	for _, stage := range marketingdto.CampaignStages {
		if err := env.repo.DeleteColumn(env.ctx, stages[stage].ID); !errors.Is(err, ErrCampaignColumnProtected) {
			t.Errorf("delete of the %s column: %v, want ErrCampaignColumnProtected", stage, err)
		}
	}
	item := env.createCampaign("In the parking lane", "live")
	if _, err := env.repo.MoveCampaign(env.ctx, item.ID, last.ID, 1, env.tenants[0].userID); err != nil {
		t.Fatal(err)
	}
	if err := env.repo.DeleteColumn(env.ctx, last.ID); !errors.Is(err, ErrCampaignColumnInUse) {
		t.Errorf("delete of a column with campaigns: %v, want ErrCampaignColumnInUse", err)
	}
	if err := env.repo.DeleteColumn(env.ctx, uuid.NewString()); !errors.Is(err, ErrCampaignColumnNotFound) {
		t.Errorf("delete of an unknown column: %v, want ErrCampaignColumnNotFound", err)
	}
	if got := env.boardNames(); !reflect.DeepEqual(got, want) {
		t.Errorf("refused deletes changed the board: %v", got)
	}

	// Reorder: reversed order works; bad payloads are refused untouched.
	board := env.board()
	ids := make([]string, len(board))
	for index, row := range board {
		ids[len(board)-1-index] = row.ID
	}
	if err := env.repo.ReorderColumns(env.ctx, ids); err != nil {
		t.Fatalf("reorder: %v", err)
	}
	want = []string{"1:Parkir", "2:Archived", "3:Completed", "4:Live", "5:In Production", "6:Planning", "7:Ideation", "8:Inbox"}
	if got := env.boardNames(); !reflect.DeepEqual(got, want) {
		t.Errorf("after reorder:\n got %v\nwant %v", got, want)
	}
	for name, payload := range map[string][]string{
		"one column missing": ids[1:],
		"duplicate id":       append(append([]string(nil), ids[1:]...), ids[1]),
		"unknown id":         append(append([]string(nil), ids[1:]...), uuid.NewString()),
		"one extra id":       append(append([]string(nil), ids...), uuid.NewString()),
	} {
		if err := env.repo.ReorderColumns(env.ctx, payload); !errors.Is(err, ErrCampaignColumnReorderInvalid) {
			t.Errorf("reorder with %s: %v, want ErrCampaignColumnReorderInvalid", name, err)
		}
	}
	if got := env.boardNames(); !reflect.DeepEqual(got, want) {
		t.Errorf("refused reorders changed the board: %v", got)
	}

	// The stage keys survived all of it, and stage lookup ignores the order.
	assertAllStagesPresent(t, env.board())
	if moved := env.createCampaign("After reorder", "ideation"); deref(moved.ColumnName) != "Ideation" {
		t.Errorf("create after reorder landed in %q", deref(moved.ColumnName))
	}

	// Column list: counts, no campaigns payload.
	columns, err := env.repo.ListColumns(env.ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, column := range columns {
		if column.Campaigns != nil {
			t.Errorf("column list must not carry campaigns (%q)", column.Name)
		}
		if column.Name == "Parkir" && column.CampaignsNo != 1 {
			t.Errorf("Parkir campaign_count = %d, want 1", column.CampaignsNo)
		}
	}
}

func TestMetaAdsIsAcceptedForCampaignsAndAdsMetrics(t *testing.T) {
	env := newCampaignTestEnv(t)

	params := env.campaignParams("Meta campaign", "live")
	params.Channel = "meta_ads"
	item, err := env.repo.CreateCampaign(env.ctx, params)
	if err != nil {
		t.Fatalf("create campaign with channel meta_ads: %v", err)
	}
	if item.Channel != "meta_ads" {
		t.Errorf("channel = %q", item.Channel)
	}
	if items, total, err := env.repo.ListCampaigns(env.ctx, ListCampaignsParams{Page: 1, PerPage: 10, Channel: "meta_ads"}); err != nil || total != 1 || len(items) != 1 {
		t.Errorf("filter by meta_ads: %d items, total %d, err %v", len(items), total, err)
	}

	metric, err := env.metrics.CreateMetric(env.ctx, UpsertAdsMetricParams{
		CampaignID:  item.ID,
		Platform:    "meta_ads",
		PeriodStart: params.StartDate,
		PeriodEnd:   params.EndDate,
		AmountSpent: 350000,
		Impressions: 12000,
		Clicks:      480,
		CreatedBy:   env.tenants[0].userID,
	})
	if err != nil {
		t.Fatalf("create ads metric with platform meta_ads: %v", err)
	}
	if metric.Platform != "meta_ads" {
		t.Errorf("platform = %q", metric.Platform)
	}

	// Every value of the shared lists passes its CHECK, so the lists and the
	// constraints cannot drift apart unnoticed.
	for _, channel := range marketingdto.CampaignChannels {
		p := env.campaignParams("Channel "+channel, "planning")
		p.Channel = channel
		if _, err := env.repo.CreateCampaign(env.ctx, p); err != nil {
			t.Errorf("channel %q rejected by the database: %v", channel, err)
		}
	}
	for _, platform := range marketingdto.AdsMetricPlatforms {
		if _, err := env.metrics.CreateMetric(env.ctx, UpsertAdsMetricParams{
			CampaignID: item.ID, Platform: platform, PeriodStart: params.StartDate, PeriodEnd: params.EndDate, CreatedBy: env.tenants[0].userID,
		}); err != nil {
			t.Errorf("platform %q rejected by the database: %v", platform, err)
		}
	}
}

// What the request validation normally stops must still come back typed when
// it reaches the database, and must not break the surrounding transaction.
func TestCampaignWriteErrorsAreTyped(t *testing.T) {
	env := newCampaignTestEnv(t)
	env.ensure()

	backwards := env.campaignParams("Backwards", "planning")
	backwards.StartDate, backwards.EndDate = backwards.EndDate, backwards.StartDate
	if _, err := env.repo.CreateCampaign(env.ctx, backwards); !errors.Is(err, ErrCampaignDateRange) {
		t.Errorf("end before start on create: %v, want ErrCampaignDateRange", err)
	}

	unknownChannel := env.campaignParams("Unknown channel", "planning")
	unknownChannel.Channel = "billboard"
	if _, err := env.repo.CreateCampaign(env.ctx, unknownChannel); !errors.Is(err, ErrCampaignChannelInvalid) {
		t.Errorf("unknown channel on create: %v, want ErrCampaignChannelInvalid", err)
	}

	missingPIC := env.campaignParams("Missing PIC", "planning")
	nobody := uuid.NewString()
	missingPIC.PICEmployeeID = &nobody
	if _, err := env.repo.CreateCampaign(env.ctx, missingPIC); !errors.Is(err, ErrCampaignPICNotFound) {
		t.Errorf("unknown PIC on create: %v, want ErrCampaignPICNotFound", err)
	}

	item := env.createCampaign("Valid", "planning")
	if _, _, err := env.repo.UpdateCampaign(env.ctx, item.ID, backwards); !errors.Is(err, ErrCampaignDateRange) {
		t.Errorf("end before start on update: %v, want ErrCampaignDateRange", err)
	}
	if _, total, err := env.repo.ListCampaigns(env.ctx, ListCampaignsParams{Page: 1, PerPage: 10}); err != nil || total != 1 {
		t.Errorf("after the refused writes: total %d, err %v; want exactly the one valid campaign", total, err)
	}
	if reloaded, err := env.repo.GetCampaignByID(env.ctx, item.ID); err != nil || reloaded.Name != "Valid" {
		t.Errorf("refused update changed the campaign: %+v, err %v", reloaded, err)
	}
}

func TestListPICOptions(t *testing.T) {
	env := newCampaignTestEnv(t)

	insertEmployee := func(name string, status string) string {
		var id string
		env.mustScan(&id, `
			INSERT INTO employees (full_name, email, position, date_joined, employment_status)
			VALUES ($1, $2, 'Staff', DATE '2025-01-06', $3)
			RETURNING id::text
		`, name, strings.ToLower(strings.ReplaceAll(name, " ", "."))+"-"+uuid.NewString()[:8]+"@example.test", status)
		return id
	}
	insertEmployee("citra Fixture", "active")
	insertEmployee("Budi Fixture", "probation")
	formerPIC := insertEmployee("Dewi Fixture", "resigned")
	insertEmployee("Eko Fixture", "terminated")

	params := env.campaignParams("Handed over", "live")
	params.PICEmployeeID = &formerPIC
	if _, err := env.repo.CreateCampaign(env.ctx, params); err != nil {
		t.Fatalf("create campaign with a PIC: %v", err)
	}

	// Another tenant's employees never show up.
	env.useTenant(1)
	insertEmployee("Andi Other Tenant", "active")
	env.useTenant(0)

	options, err := env.repo.ListPICOptions(env.ctx)
	if err != nil {
		t.Fatalf("ListPICOptions: %v", err)
	}
	var names []string
	for _, option := range options {
		names = append(names, option.FullName)
		if option.ID == "" || option.Position != "Staff" {
			t.Errorf("incomplete option: %+v", option)
		}
	}
	want := []string{"Budi Fixture", "citra Fixture", "Dewi Fixture"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("options = %v, want %v (current staff plus the former employee who still is a PIC, by name)", names, want)
	}
}

// One entry per action in the feed: the request audit rows that duplicate a
// descriptive row are left out, and an edit that changed the stage says so.
func TestListActivitiesShowsOneEntryPerAction(t *testing.T) {
	env := newCampaignTestEnv(t)
	item := env.createCampaign("With history", "planning")
	userID := env.tenants[0].userID

	insertAudit := func(action string, oldValue string, newValue string) {
		env.mustExec(`
			INSERT INTO audit_logs (user_id, action, module, resource, resource_id, old_value, new_value, created_at)
			VALUES ($1::uuid, $2, 'marketing', 'campaign', $3, NULLIF($4, '')::jsonb, NULLIF($5, '')::jsonb, clock_timestamp())
		`, userID, action, item.ID, oldValue, newValue)
	}
	insertAudit("create", "", `{"name":"With history","status":"planning"}`)
	// A move: the descriptive row, then the handler's request audit row.
	if err := env.repo.LogActivity(env.ctx, item.ID, userID, "campaign_moved", map[string]any{"column_name": "Running Ads"}); err != nil {
		t.Fatalf("LogActivity: %v", err)
	}
	insertAudit("move", "", `{"column_id":"x","position":1}`)
	// A later move whose descriptive row was not written (it is best
	// effort): its request audit row is the only trace and must stay.
	env.mustExec(`
		INSERT INTO audit_logs (user_id, action, module, resource, resource_id, new_value, created_at)
		VALUES ($1::uuid, 'move', 'marketing', 'campaign', $2, '{"column_id":"y","position":1}'::jsonb, clock_timestamp() + INTERVAL '1 hour')
	`, userID, item.ID)
	// An upload of one file: same pair.
	insertAudit("upload_attachments", "", "")
	if err := env.repo.LogActivity(env.ctx, item.ID, userID, "attachment_uploaded", map[string]any{"file_name": "brief.pdf"}); err != nil {
		t.Fatalf("LogActivity: %v", err)
	}
	// Two edits: one keeps the stage, one changes it.
	insertAudit("update", "", `{"name":"With history","status":"live"}`)
	insertAudit("update", `{"status":"live"}`, `{"name":"With history","status":"in_production"}`)

	activities, err := env.repo.ListActivities(env.ctx, item.ID)
	if err != nil {
		t.Fatalf("ListActivities: %v", err)
	}
	var descriptions []string
	for _, activity := range activities {
		descriptions = append(descriptions, activity.Description)
	}
	want := map[string]int{
		"Create":                             1,
		"Moved to Running Ads":               1,
		"Moved to another lane":              1,
		"Uploaded brief.pdf":                 1,
		"Update":                             1,
		"Updated and moved to In Production": 1,
	}
	got := map[string]int{}
	for _, description := range descriptions {
		got[description]++
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("activity feed = %v, want one each of %v", descriptions, want)
	}
}
