package hris

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/model"
	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

// TestPayslipRepositoryLifecycle runs the payslip SQL against a migrated
// database (KANTOR_TEST_DATABASE_URL) inside one rolled-back transaction:
// create, the one-active-slip rule, render claim / complete (returning the
// superseded PDF) / stale claim, sent + consumed ids, D9 anchor, void.
func TestPayslipRepositoryLifecycle(t *testing.T) {
	pool, tenantID := sequenceTestPool(t)
	repo := NewPayslipsRepository(pool, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	errRollback := errors.New("rollback")
	err := withTenantTx(ctx, pool, tenantID, false, func(ctx context.Context) error {
		// Own fixtures: the test passes on any database, not only a fresh one.
		employeeID := createFixtureEmployee(ctx, t, nil)
		run := testRunSuffix()
		bonusID, reimbursementID := "11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222"
		params := CreatePayslipParams{
			EmployeeID:       employeeID,
			PeriodYear:       2099,
			PeriodMonth:      1,
			DocNumber:        "PAY/2099/01/TEST" + run,
			PayDate:          time.Date(2099, 1, 23, 0, 0, 0, 0, time.UTC),
			AmountsEncrypted: "v1:amounts",
			PayloadEncrypted: "v1:payload",
			ReimbursementIDs: []string{reimbursementID},
			BonusIDs:         []string{bonusID},
			Warnings:         []model.PayslipWarning{{Code: "job_title_missing", Message: "Jabatan belum diisi"}},
		}
		slip, err := repo.Create(ctx, params)
		if err != nil {
			return err
		}
		if slip.Status != model.PayslipStatusDraft || slip.RenderStatus != model.DocumentRenderPending || len(slip.Warnings) != 1 || len(slip.BonusIDs) != 1 {
			t.Errorf("created = %+v", slip)
		}

		// One active slip per employee and period.
		dup := params
		dup.DocNumber = "PAY/2099/01/TEST-2" + run
		// (in a savepoint, so the expected error does not abort the test
		// transaction)
		if err := repo.WithTx(ctx, func(ctx context.Context) error {
			_, err := repo.Create(ctx, dup)
			return err
		}); !errors.Is(err, ErrPayslipActiveExists) {
			t.Errorf("duplicate active slip err = %v", err)
		}

		claim, claimed, ok, err := repo.ClaimRender(ctx, slip.ID, time.Now().Add(-5*time.Minute))
		if err != nil || !ok || claim == "" || claimed.RenderStatus != model.DocumentRenderRendering {
			t.Fatalf("claim = %q %v %v %+v", claim, ok, err, claimed)
		}
		if _, _, ok, _ := repo.ClaimRender(ctx, slip.ID, time.Now().Add(-5*time.Minute)); ok {
			t.Error("a fresh claim must not be claimed twice")
		}
		// The page check replaces the many_rows estimate with the PDF's
		// page count.
		pageCodes := []string{"many_rows", "pdf_pages"}
		pageWarnings := []model.PayslipWarning{{Code: "pdf_pages", Message: "PDF slip 2 halaman"}}
		previous, err := repo.CompleteRender(ctx, slip.ID, claim, "documents/x/payslip/a.pdf.enc", "aaa", "v", pageCodes, pageWarnings)
		if err != nil || previous != "" {
			t.Fatalf("first complete = %q, %v", previous, err)
		}
		rendered, err := repo.GetByID(ctx, slip.ID)
		if err != nil {
			return err
		}
		if len(rendered.Warnings) != 2 || rendered.Warnings[0].Code != "job_title_missing" || rendered.Warnings[1].Code != "pdf_pages" {
			t.Errorf("warnings after render = %+v", rendered.Warnings)
		}

		// Edit -> pending -> render again: the first PDF is returned as
		// superseded; the old claim no longer completes.
		if _, err := repo.UpdateDraftSnapshot(ctx, slip.ID, UpdatePayslipSnapshotParams{
			PayDate: params.PayDate, AmountsEncrypted: "v1:a2", PayloadEncrypted: "v1:p2",
			ReimbursementIDs: params.ReimbursementIDs, BonusIDs: params.BonusIDs,
		}); err != nil {
			return err
		}
		if _, err := repo.CompleteRender(ctx, slip.ID, claim, "documents/x/payslip/stale.pdf.enc", "s", "v", nil, nil); !errors.Is(err, ErrPayslipStateChanged) {
			t.Errorf("stale complete err = %v", err)
		}
		claim2, _, ok, err := repo.ClaimRender(ctx, slip.ID, time.Now().Add(-5*time.Minute))
		if err != nil || !ok {
			t.Fatalf("second claim: %v %v", ok, err)
		}
		previous, err = repo.CompleteRender(ctx, slip.ID, claim2, "documents/x/payslip/b.pdf.enc", "bbb", "v", pageCodes, nil)
		if err != nil || previous != "documents/x/payslip/a.pdf.enc" {
			t.Fatalf("second complete previous = %q, %v", previous, err)
		}

		// A draft of an earlier period reserves its items for later periods
		// only.
		if bonuses, _, err := repo.ConsumedItemIDs(ctx, employeeID, 2099, 1); err != nil || bonuses[bonusID] {
			t.Errorf("own period consumed = %v, %v", bonuses, err)
		}
		if bonuses, reimbursements, err := repo.ConsumedItemIDs(ctx, employeeID, 2099, 2); err != nil || !bonuses[bonusID] || !reimbursements[reimbursementID] {
			t.Errorf("later period consumed = %v / %v, %v", bonuses, reimbursements, err)
		}

		// A second draft (next period) holding the same bonus: no overlap
		// while the first is an idle draft; an overlap once it is being
		// e-mailed, and the e-mail freezes the first draft.
		next := params
		next.PeriodMonth, next.DocNumber, next.ReimbursementIDs = 2, "PAY/2099/02/TEST"+run, nil
		nextSlip, err := repo.Create(ctx, next)
		if err != nil {
			return err
		}
		if overlap, err := repo.OverlappingSlip(ctx, nextSlip.ID); err != nil || overlap != nil {
			t.Errorf("idle draft overlap = %+v, %v", overlap, err)
		}
		if _, err := repository.DB(ctx, nil).Exec(ctx, `
			INSERT INTO email_deliveries (kind, reference_type, reference_id, recipient, recipient_source, subject, status)
			VALUES ('payslip', 'payslip', $1::uuid, 'a@example.com', 'employee', 'Slip', 'queued')`, slip.ID); err != nil {
			return err
		}
		if overlap, err := repo.OverlappingSlip(ctx, nextSlip.ID); err != nil || overlap == nil || overlap.ID != slip.ID || !overlap.Sending {
			t.Errorf("sending overlap = %+v, %v", overlap, err)
		}
		if _, err := repo.UpdateDraftSnapshot(ctx, slip.ID, UpdatePayslipSnapshotParams{
			PayDate: params.PayDate, AmountsEncrypted: "v1:a3", PayloadEncrypted: "v1:p3",
		}); !errors.Is(err, ErrPayslipSendInFlight) {
			t.Errorf("edit while sending err = %v", err)
		}

		// MarkSent only accepts the snapshot that was e-mailed.
		if _, err := repo.MarkSent(ctx, slip.ID, time.Now(), SentSnapshot{PayloadEncrypted: "v1:payload", PDFSHA256: "bbb"}); !errors.Is(err, ErrPayslipStateChanged) {
			t.Errorf("mark sent with an old snapshot err = %v", err)
		}
		if _, err := repo.MarkSent(ctx, slip.ID, time.Now(), SentSnapshot{PayloadEncrypted: "v1:p2", PDFSHA256: "aaa"}); !errors.Is(err, ErrPayslipStateChanged) {
			t.Errorf("mark sent with an old PDF err = %v", err)
		}
		if _, err := repo.MarkSent(ctx, slip.ID, time.Now(), SentSnapshot{PayloadEncrypted: "v1:p2", PDFSHA256: "bbb"}); err != nil {
			return err
		}
		if overlap, err := repo.OverlappingSlip(ctx, nextSlip.ID); err != nil || overlap == nil || overlap.Sending {
			t.Errorf("sent overlap = %+v, %v", overlap, err)
		}
		// Sent slips are never re-rendered.
		if _, _, ok, err := repo.ClaimRender(ctx, slip.ID, time.Now().Add(time.Hour)); err != nil || ok {
			t.Errorf("claim of a sent slip = %v, %v", ok, err)
		}
		bonuses, reimbursements, err := repo.ConsumedItemIDs(ctx, employeeID, 2099, 3)
		if err != nil {
			return err
		}
		if !bonuses[bonusID] || !reimbursements[reimbursementID] {
			t.Errorf("consumed = %v / %v", bonuses, reimbursements)
		}
		anchor, err := repo.LatestSentBefore(ctx, employeeID, 2099, 2)
		if err != nil || anchor == nil || anchor.ID != slip.ID {
			t.Errorf("anchor = %+v, %v", anchor, err)
		}
		if anchor, _ := repo.LatestSentBefore(ctx, employeeID, 2099, 1); anchor != nil && anchor.ID == slip.ID {
			t.Errorf("a slip is not its own anchor: %+v", anchor)
		}

		voided, err := repo.Void(ctx, slip.ID, "", "test")
		if err != nil || voided.Status != model.PayslipStatusVoid || voided.VoidedAt == nil {
			t.Fatalf("void = %+v, %v", voided, err)
		}
		bonuses, _, _ = repo.ConsumedItemIDs(ctx, employeeID, 2099, 1)
		if bonuses[bonusID] {
			t.Error("a void slip must not consume its items")
		}
		revision, err := repo.MaxRevision(ctx, employeeID, 2099, 1)
		if err != nil || revision != 0 {
			t.Errorf("max revision = %d, %v", revision, err)
		}
		// After the void a new active slip may exist.
		reissue := params
		reissue.DocNumber, reissue.Revision = "PAY/2099/01/TEST-R1"+run, 1
		if _, err := repo.Create(ctx, reissue); err != nil {
			t.Errorf("reissue create: %v", err)
		}
		return errRollback
	})
	if err != nil && !errors.Is(err, errRollback) {
		t.Fatal(err)
	}
}
