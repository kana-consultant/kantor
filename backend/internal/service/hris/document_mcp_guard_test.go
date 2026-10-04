package hris

import (
	"errors"
	"strings"
	"testing"

	hrisdto "github.com/kana-consultant/kantor/backend/internal/dto/hris"
	"github.com/kana-consultant/kantor/backend/internal/model"
)

// payslipMCPViewer is the same HR user calling through the MCP tool surface.
var payslipMCPViewer = DocumentViewer{ActorID: payslipTestActor, CanViewIdentity: true, ViaMCP: true}

func TestGuardSend(t *testing.T) {
	login := ResolvedRecipient{Address: "staff.ops@kantor.local", Source: model.EmailRecipientSourceLogin, Linked: true}
	employee := ResolvedRecipient{Address: "gita@kantor.local", Source: model.EmailRecipientSourceEmployee}
	personal := ResolvedRecipient{Address: "budi.pribadi@example.com", Source: model.EmailRecipientSourcePersonal, Linked: true}
	web := DocumentViewer{ActorID: "u"}
	mcp := DocumentViewer{ActorID: "u", ViaMCP: true}

	tests := []struct {
		name      string
		viewer    DocumentViewer
		recipient ResolvedRecipient
		expected  string
		want      error
	}{
		{"web without expected", web, employee, "", nil},
		{"web expected matches", web, employee, "gita@kantor.local", nil},
		{"web expected differs", web, employee, "other@kantor.local", ErrDocumentRecipientMismatch},
		{"web may mail a personal address", web, personal, "", nil},
		{"mcp login confirmed", mcp, login, "staff.ops@kantor.local", nil},
		{"mcp login confirmed, case and spaces", mcp, login, "  Staff.Ops@Kantor.Local ", nil},
		{"mcp login without expected", mcp, login, "", ErrDocumentExpectedRecipientRequired},
		{"mcp login, blank expected", mcp, login, "   ", ErrDocumentExpectedRecipientRequired},
		{"mcp login, other address", mcp, login, "attacker@evil.test", ErrDocumentRecipientMismatch},
		{"mcp employee address even when confirmed", mcp, employee, "gita@kantor.local", ErrDocumentRecipientRestricted},
		{"mcp personal address even when confirmed", mcp, personal, "budi.pribadi@example.com", ErrDocumentRecipientRestricted},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.viewer.guardSend(tc.recipient, tc.expected); !errors.Is(err, tc.want) {
				t.Errorf("guardSend = %v, want %v", err, tc.want)
			}
		})
	}
}

// Identity data stays off the MCP surface: a personal address is masked even
// for a caller who may see it in the web app, and no access row is written.
func TestPersonalAddressIsMaskedViaMCP(t *testing.T) {
	const personal = "budi.pribadi@example.com"

	web := newPersonalReveal(DocumentViewer{ActorID: "u", CanViewIdentity: true})
	if got := web.address("e-budi", personal, model.EmailRecipientSourcePersonal); got != personal || !web.employees["e-budi"] {
		t.Fatalf("web app: got %q, logged %v", got, web.employees)
	}

	mcp := newPersonalReveal(DocumentViewer{ActorID: "u", CanViewIdentity: true, ViaMCP: true})
	if got := mcp.address("e-budi", personal, model.EmailRecipientSourcePersonal); got == personal || got != MaskEmail(personal) {
		t.Fatalf("MCP: personal address not masked: %q", got)
	}
	if len(mcp.employees) != 0 {
		t.Fatalf("MCP: nothing was revealed, so nothing may be access-logged: %v", mcp.employees)
	}
	// Login and employee addresses are not identity data.
	if got := mcp.address("e-budi", "staff.ops@kantor.local", model.EmailRecipientSourceLogin); got != "staff.ops@kantor.local" {
		t.Fatalf("MCP: login address changed: %q", got)
	}
}

// Two ready September drafts: Budi (linked account) and Gita (no account, so
// her slip would go to employees.email).
func mcpPayslipFixture(t *testing.T) (f *payslipFixture, budiID string, gitaID string) {
	t.Helper()
	f = newPayslipFixture(t)
	f.compensation.salaries["e-gita"] = model.SalaryRecord{ID: "s-gita", BaseSalary: 5_000_000}
	generated := f.generate(t, 2026, 9, "e-budi", "e-gita")
	if len(generated.Generated) != 2 {
		t.Fatalf("generated = %+v", generated)
	}
	for _, item := range generated.Generated {
		f.repo.setRenderReady(*item.PayslipID)
		if item.EmployeeID == "e-budi" {
			budiID = *item.PayslipID
		} else {
			gitaID = *item.PayslipID
		}
	}
	return f, budiID, gitaID
}

func (f *payslipFixture) nothingMailed(t *testing.T) {
	t.Helper()
	if len(f.sender.delivered) != 0 || len(f.sender.queued) != 0 {
		t.Fatalf("nothing may be queued or delivered, got %d queued, %d delivered", len(f.sender.queued), len(f.sender.delivered))
	}
}

func TestPayslipSendViaMCPNeedsConfirmedLoginAddress(t *testing.T) {
	f, budiID, gitaID := mcpPayslipFixture(t)

	if _, err := f.service.Send(f.ctx, payslipMCPViewer, budiID, hrisdto.SendPayslipRequest{}); !errors.Is(err, ErrDocumentExpectedRecipientRequired) {
		t.Fatalf("send without the approved address err = %v", err)
	}
	if _, err := f.service.Send(f.ctx, payslipMCPViewer, budiID, hrisdto.SendPayslipRequest{ExpectedRecipient: "budi@example.com"}); !errors.Is(err, ErrDocumentRecipientMismatch) {
		t.Fatalf("send with another address err = %v", err)
	}
	// Budi's employee e-mail, picked explicitly and confirmed: still not a
	// login address.
	if _, err := f.service.Send(f.ctx, payslipMCPViewer, budiID, hrisdto.SendPayslipRequest{RecipientSource: "employee", ExpectedRecipient: "budi@example.com"}); !errors.Is(err, ErrDocumentRecipientRestricted) {
		t.Fatalf("send to the employee e-mail err = %v", err)
	}
	// Gita has no account: her slip goes to employees.email, which an
	// employee-edit tool can change.
	if _, err := f.service.Send(f.ctx, payslipMCPViewer, gitaID, hrisdto.SendPayslipRequest{ExpectedRecipient: "gita.permatasari@kantor.local"}); !errors.Is(err, ErrDocumentRecipientRestricted) {
		t.Fatalf("send to an unlinked employee err = %v", err)
	}
	gita := f.repo.employees["e-gita"]
	gita.Email = "payroll-collector@evil.test"
	f.repo.employees["e-gita"] = gita
	if _, err := f.service.Send(f.ctx, payslipMCPViewer, gitaID, hrisdto.SendPayslipRequest{ExpectedRecipient: "payroll-collector@evil.test"}); !errors.Is(err, ErrDocumentRecipientRestricted) {
		t.Fatalf("send to a changed employee e-mail err = %v", err)
	}
	f.nothingMailed(t)

	sent, err := f.service.Send(f.ctx, payslipMCPViewer, budiID, hrisdto.SendPayslipRequest{ExpectedRecipient: " Staff.Ops@Kantor.Local"})
	if err != nil {
		t.Fatalf("confirmed send: %v", err)
	}
	if !sent.Response.Sent || len(f.sender.delivered) != 1 {
		t.Fatalf("confirmed send result = %+v, %d mails", sent.Response, len(f.sender.delivered))
	}
	if got := f.sender.delivered[0]; got.Recipient != "staff.ops@kantor.local" || got.RecipientSource != model.EmailRecipientSourceLogin {
		t.Fatalf("mailed %s (%s)", got.Recipient, got.RecipientSource)
	}
	if sent.Audit["via"] != "mcp" {
		t.Errorf("send audit via = %v", sent.Audit["via"])
	}
}

// A refused send returns no amounts, so it must not write a salary-access
// row either.
func TestRefusedPayslipSendWritesNoAccessRow(t *testing.T) {
	f, budiID, gitaID := mcpPayslipFixture(t)
	before := len(f.compensation.accessLog)

	if _, err := f.service.Send(f.ctx, payslipMCPViewer, gitaID, hrisdto.SendPayslipRequest{ExpectedRecipient: "gita.permatasari@kantor.local"}); !errors.Is(err, ErrDocumentRecipientRestricted) {
		t.Fatalf("err = %v", err)
	}
	if _, err := f.service.Send(f.ctx, payslipMCPViewer, budiID, hrisdto.SendPayslipRequest{}); !errors.Is(err, ErrDocumentExpectedRecipientRequired) {
		t.Fatalf("err = %v", err)
	}
	if got := f.compensation.accessLog[before:]; len(got) != 0 {
		t.Fatalf("refused sends logged salary access: %v", got)
	}
}

// The web app's audit rows carry no marker.
func TestWebSendAuditHasNoVia(t *testing.T) {
	f, budiID, _ := mcpPayslipFixture(t)
	sent, err := f.service.Send(f.ctx, payslipTestViewer, budiID, hrisdto.SendPayslipRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, has := sent.Audit["via"]; has {
		t.Errorf("web send audit = %v", sent.Audit)
	}
}

// The web app keeps working without expected_recipient, and honours it when
// it is given.
func TestPayslipSendExpectedRecipientIsOptionalForTheWebApp(t *testing.T) {
	f, _, gitaID := mcpPayslipFixture(t)

	if _, err := f.service.Send(f.ctx, payslipTestViewer, gitaID, hrisdto.SendPayslipRequest{ExpectedRecipient: "someone.else@kantor.local"}); !errors.Is(err, ErrDocumentRecipientMismatch) {
		t.Fatalf("mismatch err = %v", err)
	}
	f.nothingMailed(t)
	if _, err := f.service.Send(f.ctx, payslipTestViewer, gitaID, hrisdto.SendPayslipRequest{ExpectedRecipient: "gita.permatasari@kantor.local"}); err != nil {
		t.Fatalf("matching send: %v", err)
	}
	if len(f.sender.delivered) != 1 || f.sender.delivered[0].RecipientSource != model.EmailRecipientSourceEmployee {
		t.Fatalf("delivered = %+v", f.sender.delivered)
	}
}

func TestPayslipBatchViaMCP(t *testing.T) {
	f, budiID, gitaID := mcpPayslipFixture(t)
	ids := []string{budiID, gitaID}

	preview, err := f.service.SendPreview(f.ctx, payslipMCPViewer, hrisdto.PayslipSendPreviewRequest{IDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Recipients) != 1 || preview.Recipients[0].PayslipID != budiID || preview.Recipients[0].Recipient != "staff.ops@kantor.local" {
		t.Fatalf("preview recipients = %+v", preview.Recipients)
	}
	if len(preview.Skipped) != 1 || preview.Skipped[0].Code != "mcp_recipient_not_login" || optionalText(preview.Skipped[0].PayslipID) != gitaID {
		t.Fatalf("preview skipped = %+v", preview.Skipped)
	}

	refused := []struct {
		name     string
		expected map[string]string
		want     error
	}{
		{"no addresses", nil, ErrDocumentExpectedRecipientsRequired},
		{"empty map", map[string]string{}, ErrDocumentExpectedRecipientsRequired},
		{"slip not listed", map[string]string{gitaID: "gita.permatasari@kantor.local"}, ErrDocumentRecipientMismatch},
		{"blank address", map[string]string{budiID: " "}, ErrDocumentRecipientMismatch},
		{"other address", map[string]string{budiID: "attacker@evil.test"}, ErrDocumentRecipientMismatch},
	}
	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.service.PrepareBatch(f.ctx, payslipMCPViewer, hrisdto.SendPayslipBatchRequest{IDs: ids, ExpectedRecipients: tc.expected})
			if !errors.Is(err, tc.want) {
				t.Fatalf("PrepareBatch err = %v, want %v", err, tc.want)
			}
			f.nothingMailed(t)
			// The message names the slip to fix.
			if errors.Is(err, ErrDocumentRecipientMismatch) && !strings.Contains(err.Error(), budiID) {
				t.Errorf("mismatch message does not name the slip: %v", err)
			}
		})
	}

	prepared, err := f.service.PrepareBatch(f.ctx, payslipMCPViewer, hrisdto.SendPayslipBatchRequest{
		IDs: ids,
		// Listing Gita does not make her sendable through MCP.
		ExpectedRecipients: map[string]string{budiID: "staff.ops@kantor.local", gitaID: "gita.permatasari@kantor.local"},
	})
	if err != nil {
		t.Fatalf("confirmed batch: %v", err)
	}
	if len(prepared.Response.Queued) != 1 || prepared.Response.Queued[0].PayslipID != budiID || len(f.sender.queued) != 1 {
		t.Fatalf("queued = %+v", prepared.Response.Queued)
	}
	if len(prepared.Response.Skipped) != 1 || prepared.Response.Skipped[0].Code != "mcp_recipient_not_login" {
		t.Fatalf("skipped = %+v", prepared.Response.Skipped)
	}
	// Both audit rows of the send say it came through MCP: the request row
	// and the outcome row the background sender writes.
	if got := prepared.Requested[0].Values["via"]; got != "mcp" {
		t.Errorf("send_requested audit via = %v", got)
	}
	if got := prepared.jobs[0].AuditValue["via"]; got != "mcp" {
		t.Errorf("batch outcome audit via = %v", got)
	}
}

// Outside MCP a batch still mails unlinked employees, and an optional
// expected_recipients map is enforced for every slip that would be sent.
func TestPayslipBatchExpectedRecipientsForTheWebApp(t *testing.T) {
	f, budiID, gitaID := mcpPayslipFixture(t)
	ids := []string{budiID, gitaID}

	_, err := f.service.PrepareBatch(f.ctx, payslipTestViewer, hrisdto.SendPayslipBatchRequest{
		IDs:                ids,
		ExpectedRecipients: map[string]string{budiID: "staff.ops@kantor.local", gitaID: "old.address@kantor.local"},
	})
	if !errors.Is(err, ErrDocumentRecipientMismatch) {
		t.Fatalf("stale preview err = %v", err)
	}
	f.nothingMailed(t)

	prepared, err := f.service.PrepareBatch(f.ctx, payslipTestViewer, hrisdto.SendPayslipBatchRequest{
		IDs:                ids,
		ExpectedRecipients: map[string]string{budiID: "staff.ops@kantor.local", gitaID: "gita.permatasari@kantor.local"},
	})
	if err != nil {
		t.Fatalf("matching batch: %v", err)
	}
	if len(prepared.Response.Queued) != 2 {
		t.Fatalf("queued = %+v", prepared.Response.Queued)
	}
}

func TestContractSendViaMCP(t *testing.T) {
	f := newContractFixture(t)
	created, _, err := f.service.Create(f.ctx, contractHR, budiPKWTRequest(f.budi))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.service.Generate(f.ctx, contractHR, created.ID); err != nil {
		t.Fatal(err)
	}
	f.repo.setRenderReady(created.ID)

	mcp := contractHR
	mcp.ViaMCP = true
	const login = "staff.ops@kantor.local"

	refused := []struct {
		name  string
		input hrisdto.SendContractRequest
		want  error
	}{
		{"no approved address", hrisdto.SendContractRequest{}, ErrDocumentExpectedRecipientRequired},
		{"other address", hrisdto.SendContractRequest{ExpectedRecipient: "attacker@evil.test"}, ErrDocumentRecipientMismatch},
		{"employee e-mail", hrisdto.SendContractRequest{RecipientSource: "employee", ExpectedRecipient: "budi@example.com"}, ErrDocumentRecipientRestricted},
		{"cc on the company domain", hrisdto.SendContractRequest{ExpectedRecipient: login, Cc: []string{"legal@contoh.co.id"}}, ErrDocumentCcRestricted},
	}
	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := f.service.Send(f.ctx, mcp, created.ID, tc.input); !errors.Is(err, tc.want) {
				t.Fatalf("Send err = %v, want %v", err, tc.want)
			}
			if len(f.sender.delivered) != 0 || len(f.sender.queued) != 0 {
				t.Fatalf("nothing may be mailed, got %d queued, %d delivered", len(f.sender.queued), len(f.sender.delivered))
			}
		})
	}

	sent, err := f.service.Send(f.ctx, mcp, created.ID, hrisdto.SendContractRequest{ExpectedRecipient: login})
	if err != nil {
		t.Fatalf("confirmed send: %v", err)
	}
	if !sent.Response.Sent || len(f.sender.delivered) != 1 {
		t.Fatalf("confirmed send = %+v, %d mails", sent.Response, len(f.sender.delivered))
	}
	if got := f.sender.delivered[0]; got.Recipient != login || len(got.Cc) != 0 {
		t.Fatalf("mailed %s cc %v", got.Recipient, got.Cc)
	}
	if sent.Audit["via"] != "mcp" {
		t.Errorf("contract send audit via = %v", sent.Audit["via"])
	}
}
