package hris

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/model"
)

// TestContractRepositoryLifecycle runs the contract SQL against a migrated
// database (KANTOR_TEST_DATABASE_URL) inside one rolled-back transaction:
// create, the edit guard (expected status/revision), the snapshot keeping
// its numbers, render claim / complete (returning the superseded PDFs),
// mark sent, status, the renewal chain, the payslip lookup and the
// employee delete refusal.
func TestContractRepositoryLifecycle(t *testing.T) {
	pool, tenantID := sequenceTestPool(t)
	repo := NewContractsRepository(pool, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	errRollback := errors.New("rollback")
	err := withTenantTx(ctx, pool, tenantID, false, func(ctx context.Context) error {
		// Own fixtures: the test passes on any database, not only a fresh one.
		employeeID := createFixtureEmployee(ctx, t, nil)
		run := testRunSuffix()
		pkwtNumber, ndaNumber := "007/PKWT/TST"+run+"/I/2099", "007/NDA-HKI/TST"+run+"/I/2099"
		start := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(2099, 12, 31, 0, 0, 0, 0, time.UTC)
		terms := ContractTerms{
			ContractType:         model.ContractTypePKWT,
			StartDate:            start,
			EndDate:              &end,
			JobTitle:             "Backend Engineer",
			WorkLocation:         "Jakarta",
			WorkMode:             model.ContractWorkModeHybrid,
			JobDescription:       "membangun layanan",
			WorkDays:             "Senin–Jumat",
			WorkHours:            "09.00–18.00",
			WeeklyHours:          40,
			NoticeDays:           30,
			IncidentReportHours:  24,
			NonSolicitMonths:     12,
			ConfidentialityYears: 3,
			Benefits:             []model.ContractBenefit{{Name: "Laptop", Value: "Pinjam pakai"}},
		}
		contract, err := repo.Create(ctx, CreateContractParams{EmployeeID: employeeID, Terms: terms})
		if err != nil {
			return err
		}
		if contract.Status != model.ContractStatusDraft || len(contract.Benefits) != 1 || contract.EndDate == nil || !contract.EndDate.Equal(end) {
			t.Fatalf("created = %+v", contract)
		}

		// The PKWT end-date check holds in the database too.
		noEnd := terms
		noEnd.EndDate = nil
		if err := repo.WithTx(ctx, func(ctx context.Context) error {
			_, err := repo.Create(ctx, CreateContractParams{EmployeeID: employeeID, Terms: noEnd})
			return err
		}); err == nil {
			t.Error("a PKWT without end date must be refused")
		}

		documentDate := time.Date(2099, 1, 2, 0, 0, 0, 0, time.UTC)
		snapshot := ContractSnapshotParams{
			ExpectStatus: model.ContractStatusDraft, SeqNo: 7, DocNumber: pkwtNumber, NDADocNumber: ndaNumber,
			DocumentDate: documentDate, DocumentCity: "Jakarta", PayloadEncrypted: "v1:payload", TemplateVersion: "v", GeneratedBy: "", RenderStatus: model.DocumentRenderPending,
		}
		generated, err := repo.RecordSnapshot(ctx, contract.ID, snapshot)
		if err != nil {
			return err
		}
		if generated.Status != model.ContractStatusGenerated || generated.SeqNo == nil || *generated.SeqNo != 7 || generated.RenderStatus != model.DocumentRenderPending {
			t.Fatalf("generated = %+v", generated)
		}

		pending, err := repo.ListRenderPending(ctx, time.Now().Add(-5*time.Minute))
		if err != nil || !containsString(pending, contract.ID) {
			t.Fatalf("pending = %v, %v", pending, err)
		}
		claim, claimed, ok, err := repo.ClaimRender(ctx, contract.ID, time.Now().Add(-5*time.Minute))
		if err != nil || !ok || claim == "" || claimed.RenderStatus != model.DocumentRenderRendering {
			t.Fatalf("claim = %q %v %v", claim, ok, err)
		}
		superseded, err := repo.CompleteRender(ctx, contract.ID, claim, "documents/x/contract/p1.pdf.enc", "documents/x/contract/n1.pdf.enc", []string{"a", "b"}, "v")
		if err != nil || len(superseded) != 0 {
			t.Fatalf("first complete = %v, %v", superseded, err)
		}

		if _, err := repo.MarkSent(ctx, contract.ID, time.Now(), ContractSentSnapshot{Revision: 0, PayloadEncrypted: "v1:other"}); !errors.Is(err, ErrContractStateChanged) {
			t.Errorf("mark sent with another payload err = %v", err)
		}
		sent, err := repo.MarkSent(ctx, contract.ID, time.Now(), ContractSentSnapshot{Revision: 0, PayloadEncrypted: "v1:payload", PDFSHA256: []string{"a", "b"}})
		if err != nil || sent.Status != model.ContractStatusSent || sent.LastSentAt == nil {
			t.Fatalf("sent = %+v, %v", sent.Status, err)
		}

		// Edit after send: guarded by the expected status and revision.
		if _, err := repo.UpdateTerms(ctx, contract.ID, UpdateContractTermsParams{ExpectStatus: model.ContractStatusDraft, Status: model.ContractStatusDraft, Terms: terms}); !errors.Is(err, ErrContractStateChanged) {
			t.Errorf("stale edit err = %v", err)
		}
		revisedTerms := terms
		revisedTerms.JobDescription = "membangun layanan dan API"
		revisedTerms.DocumentDate = &documentDate
		revised, err := repo.UpdateTerms(ctx, contract.ID, UpdateContractTermsParams{ExpectStatus: model.ContractStatusSent, ExpectRevision: 0, Status: model.ContractStatusDraft, Revision: 1, Terms: revisedTerms})
		if err != nil {
			return err
		}
		if revised.Status != model.ContractStatusDraft || revised.Revision != 1 || revised.RenderStatus != model.DocumentRenderNone || revised.DocNumber == nil || *revised.DocNumber != pkwtNumber {
			t.Fatalf("revised = %+v", revised)
		}

		// Regenerate: the numbers stay (COALESCE), the old PDFs are returned
		// as superseded.
		snapshot.ExpectRevision = 1
		snapshot.SeqNo, snapshot.DocNumber, snapshot.NDADocNumber = 99, "099/X"+run, "099/Y"+run
		regenerated, err := repo.RecordSnapshot(ctx, contract.ID, snapshot)
		if err != nil || *regenerated.DocNumber != pkwtNumber || *regenerated.SeqNo != 7 {
			t.Fatalf("regenerated = %v, %v", regenerated.DocNumber, err)
		}
		claim, _, _, err = repo.ClaimRender(ctx, contract.ID, time.Now().Add(-5*time.Minute))
		if err != nil {
			return err
		}
		superseded, err = repo.CompleteRender(ctx, contract.ID, claim, "documents/x/contract/p2.pdf.enc", "documents/x/contract/n2.pdf.enc", []string{"c", "d"}, "v")
		if err != nil || len(superseded) != 2 || superseded[0] != "documents/x/contract/p1.pdf.enc" {
			t.Fatalf("second complete = %v, %v", superseded, err)
		}
		if _, err := repo.CompleteRender(ctx, contract.ID, claim, "a", "b", []string{"e", "f"}, "v"); !errors.Is(err, ErrContractStateChanged) {
			t.Errorf("complete with a used claim err = %v", err)
		}

		signedAt := time.Date(2099, 1, 3, 0, 0, 0, 0, time.UTC)
		if _, err := repo.SetStatus(ctx, contract.ID, []string{model.ContractStatusSent}, model.ContractStatusSigned, &signedAt, nil, nil); !errors.Is(err, ErrContractStateChanged) {
			t.Errorf("signed from the wrong status err = %v", err)
		}
		signed, err := repo.SetStatus(ctx, contract.ID, []string{model.ContractStatusGenerated, model.ContractStatusSent}, model.ContractStatusSigned, &signedAt, nil, nil)
		if err != nil || signed.SignedAt == nil || !signed.SignedAt.Equal(signedAt) {
			t.Fatalf("signed = %+v, %v", signed.SignedAt, err)
		}

		active, ok, err := repo.ActiveForPeriod(ctx, employeeID, time.Date(2099, 6, 1, 0, 0, 0, 0, time.UTC), time.Date(2099, 6, 30, 0, 0, 0, 0, time.UTC))
		if err != nil || !ok || active.ID != contract.ID {
			t.Errorf("active = %v %v %v", active.ID, ok, err)
		}

		nextStart := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
		nextEnd := time.Date(2100, 12, 31, 0, 0, 0, 0, time.UTC)
		renewalTerms := terms
		renewalTerms.StartDate, renewalTerms.EndDate = nextStart, &nextEnd
		previous := contract.ID
		renewal, err := repo.Create(ctx, CreateContractParams{EmployeeID: employeeID, PreviousContractID: &previous, Terms: renewalTerms})
		if err != nil {
			return err
		}
		if id, err := repo.RenewalOf(ctx, contract.ID); err != nil || id != renewal.ID {
			t.Errorf("renewal of = %q, %v", id, err)
		}
		links, err := repo.ChainLinks(ctx, employeeID)
		linked := map[string]bool{}
		for _, link := range links {
			if link.ID == renewal.ID && link.PreviousContractID != nil && *link.PreviousContractID == contract.ID {
				linked[link.ID] = true
			}
			if link.ID == contract.ID {
				linked[link.ID] = true
			}
		}
		if err != nil || !linked[renewal.ID] || !linked[contract.ID] {
			t.Errorf("chain links = %+v, %v", links, err)
		}

		// The edit guard waits for a lock held by generate (GetForUpdate).
		if locked, err := repo.GetForUpdate(ctx, renewal.ID); err != nil || locked.ID != renewal.ID {
			t.Errorf("get for update = %v, %v", locked.ID, err)
		}

		// A contract ended (Akhiri) during the month still counts for it.
		endedAt := time.Date(2099, 6, 10, 0, 0, 0, 0, time.UTC)
		if _, err := repo.SetStatus(ctx, contract.ID, []string{model.ContractStatusSigned}, model.ContractStatusEnded, nil, &endedAt, nil); err != nil {
			return err
		}
		active, ok, err = repo.ActiveForPeriod(ctx, employeeID, time.Date(2099, 6, 1, 0, 0, 0, 0, time.UTC), time.Date(2099, 6, 30, 0, 0, 0, 0, time.UTC))
		if err != nil || !ok || active.ID != contract.ID {
			t.Errorf("ended mid-month active = %v %v %v", active.ID, ok, err)
		}
		if _, ok, err := repo.ActiveForPeriod(ctx, employeeID, time.Date(2099, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2099, 7, 31, 0, 0, 0, 0, time.UTC)); err != nil || ok {
			t.Errorf("the month after Akhiri has an active contract: %v, %v", ok, err)
		}

		rows, err := repo.List(ctx, ContractListFilter{EmployeeID: employeeID, Search: pkwtNumber})
		if err != nil || len(rows) != 1 || rows[0].Contract.ID != contract.ID || rows[0].EmployeeName == "" {
			t.Errorf("list = %d rows, %v", len(rows), err)
		}

		// Legal records: the employee cannot be deleted (409 upstream).
		employees := NewEmployeesRepository(pool, nil)
		if err := repo.WithTx(ctx, func(ctx context.Context) error {
			return employees.DeleteEmployee(ctx, employeeID)
		}); !errors.Is(err, ErrEmployeeHasDocuments) {
			t.Errorf("delete employee with contracts err = %v", err)
		}
		return errRollback
	})
	if err != nil && !errors.Is(err, errRollback) {
		t.Fatal(err)
	}
}
