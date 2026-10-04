package hris

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/kana-consultant/kantor/backend/internal/clientvia"
	hrisdto "github.com/kana-consultant/kantor/backend/internal/dto/hris"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/model"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
	hrisservice "github.com/kana-consultant/kantor/backend/internal/service/hris"
)

func requestVia(value string, set bool) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/hris/payslips/x/send", nil)
	if set {
		r.Header.Set(clientvia.Header, value)
	}
	return r
}

// The MCP send rules hang on this one flag: it must follow the marker the
// MCP server puts on its requests, and nothing else.
func TestDocumentViewersFollowTheMCPMarker(t *testing.T) {
	principal := platformmiddleware.Principal{UserID: "u-1", IsSuperAdmin: true}
	tests := []struct {
		name  string
		value string
		set   bool
		want  bool
	}{
		{"no header (web app)", "", false, false},
		{"empty", "", true, false},
		{"mcp", "mcp", true, true},
		{"upper case", "MCP", true, true},
		{"padded", "  mcp ", true, true},
		{"another client", "web", true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := requestVia(tc.value, tc.set)
			if got := clientvia.IsMCP(r); got != tc.want {
				t.Errorf("clientvia.IsMCP = %v, want %v", got, tc.want)
			}
			viewer := documentViewer(r, principal)
			if viewer.ViaMCP != tc.want {
				t.Errorf("documentViewer.ViaMCP = %v, want %v", viewer.ViaMCP, tc.want)
			}
			if viewer.ActorID != "u-1" || !viewer.CanViewIdentity {
				t.Errorf("viewer lost its identity: %+v", viewer)
			}
			if got := contractViewer(r, principal).ViaMCP; got != tc.want {
				t.Errorf("contractViewer.ViaMCP = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMCPSendErrorsHaveTheirOwnCodes(t *testing.T) {
	tests := []struct {
		err    error
		status int
		code   string
		detail string
	}{
		{hrisservice.ErrDocumentExpectedRecipientRequired, http.StatusBadRequest, "VALIDATION_ERROR", "expected_recipient"},
		{hrisservice.ErrDocumentExpectedRecipientsRequired, http.StatusBadRequest, "VALIDATION_ERROR", "expected_recipients"},
		{hrisservice.ErrDocumentRecipientMismatch, http.StatusConflict, "RECIPIENT_MISMATCH", ""},
		{fmt.Errorf("%w (slip abc)", hrisservice.ErrDocumentRecipientMismatch), http.StatusConflict, "RECIPIENT_MISMATCH", ""},
		{hrisservice.ErrDocumentRecipientRestricted, http.StatusConflict, "MCP_RECIPIENT_NOT_ALLOWED", ""},
		{hrisservice.ErrDocumentCcRestricted, http.StatusConflict, "MCP_CC_NOT_ALLOWED", "cc"},
	}
	for _, tc := range tests {
		t.Run(tc.err.Error(), func(t *testing.T) {
			// Payslips map through writeDocumentError; contracts fall through
			// to it for everything that is not a contract error.
			for name, write := range map[string]func(http.ResponseWriter){
				"payslips":  func(w http.ResponseWriter) { writeDocumentError(context.Background(), w, tc.err, "fallback") },
				"contracts": func(w http.ResponseWriter) { (&ContractsHandler{}).writeError(context.Background(), w, tc.err) },
			} {
				recorder := httptest.NewRecorder()
				write(recorder)
				var body struct {
					Error struct {
						Code    string            `json:"code"`
						Message string            `json:"message"`
						Details map[string]string `json:"details"`
					} `json:"error"`
				}
				if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
					t.Fatalf("%s: invalid body %s", name, recorder.Body.String())
				}
				if recorder.Code != tc.status || body.Error.Code != tc.code {
					t.Errorf("%s: got %d %s, want %d %s", name, recorder.Code, body.Error.Code, tc.status, tc.code)
				}
				if body.Error.Message != tc.err.Error() {
					t.Errorf("%s: message %q, want %q", name, body.Error.Message, tc.err.Error())
				}
				if tc.detail != "" && body.Error.Details[tc.detail] == "" {
					t.Errorf("%s: details %v lack %q", name, body.Error.Details, tc.detail)
				}
			}
		})
	}
}

// Through MCP an employee's e-mail cannot be changed: for an employee without
// an account it decides which user account the employee is linked to, and
// document e-mail then goes to that account.
func TestEmployeeEmailChanges(t *testing.T) {
	stored := model.Employee{Email: "gita.permatasari@kantor.local"}
	tests := []struct {
		email string
		want  bool
	}{
		{"gita.permatasari@kantor.local", false},
		{"Gita.Permatasari@Kantor.Local", false},
		{"  gita.permatasari@kantor.local ", false},
		{"other.user@kantor.local", true},
		{"", true},
	}
	for _, tc := range tests {
		if got := employeeEmailChanges(stored, hrisdto.UpdateEmployeeRequest{Email: tc.email}); got != tc.want {
			t.Errorf("employeeEmailChanges(%q) = %v, want %v", tc.email, got, tc.want)
		}
	}
}

// stubEmployeesRepo backs a real EmployeesService so the handler runs end to
// end without a database.
type stubEmployeesRepo struct {
	stored  model.Employee
	getErr  error
	updates int
}

func (s *stubEmployeesRepo) CreateEmployee(context.Context, hrisrepo.UpsertEmployeeParams) (model.Employee, error) {
	return model.Employee{}, nil
}
func (s *stubEmployeesRepo) ListEmployees(context.Context, hrisrepo.ListEmployeesParams) ([]model.Employee, int64, error) {
	return nil, 0, nil
}
func (s *stubEmployeesRepo) GetEmployeeByID(context.Context, string) (model.Employee, error) {
	return s.stored, s.getErr
}
func (s *stubEmployeesRepo) GetEmployeeByUserID(context.Context, string) (model.Employee, error) {
	return s.stored, s.getErr
}
func (s *stubEmployeesRepo) UpdateEmployee(_ context.Context, _ string, params hrisrepo.UpsertEmployeeParams) (model.Employee, error) {
	s.updates++
	updated := s.stored
	updated.Email = params.Email
	return updated, nil
}
func (s *stubEmployeesRepo) UpdateEmployeeAvatar(context.Context, string, *string) (model.Employee, error) {
	return s.stored, nil
}
func (s *stubEmployeesRepo) DeleteEmployee(context.Context, string) error { return nil }

func updateEmployeeThrough(t *testing.T, repo *stubEmployeesRepo, viaMCP bool, email string) *httptest.ResponseRecorder {
	t.Helper()
	handler := NewEmployeesHandler(hrisservice.NewEmployeesService(repo), nil, t.TempDir(), nil)
	body := fmt.Sprintf(`{"full_name":"Gita Permatasari","email":%q,"position":"Outsourcing","date_joined":"2025-01-06","employment_status":"active"}`, email)
	r := httptest.NewRequest(http.MethodPut, "/api/v1/hris/employees/e-gita", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if viaMCP {
		r.Header.Set(clientvia.Header, clientvia.MCP)
	}
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("employeeID", "e-gita")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, routeCtx))
	w := httptest.NewRecorder()
	handler.updateEmployee(w, r)
	return w
}

func TestUpdateEmployeeRefusesEmailChangeThroughMCP(t *testing.T) {
	stored := model.Employee{ID: "e-gita", FullName: "Gita Permatasari", Email: "gita.permatasari@kantor.local", Position: "Outsourcing", EmploymentStatus: "active"}

	// Through MCP: a different e-mail is refused before anything is written.
	repo := &stubEmployeesRepo{stored: stored}
	w := updateEmployeeThrough(t, repo, true, "other.user@kantor.local")
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "EMPLOYEE_EMAIL_LOCKED") || repo.updates != 0 {
		t.Fatalf("MCP e-mail change: %d %s, %d updates", w.Code, w.Body.String(), repo.updates)
	}

	// Through MCP: the same e-mail (other fields may change) goes through.
	repo = &stubEmployeesRepo{stored: stored}
	if w := updateEmployeeThrough(t, repo, true, "Gita.Permatasari@kantor.local"); w.Code != http.StatusOK || repo.updates != 1 {
		t.Fatalf("MCP update without e-mail change: %d %s, %d updates", w.Code, w.Body.String(), repo.updates)
	}

	// Through MCP: fail closed when the stored record cannot be read.
	repo = &stubEmployeesRepo{stored: stored, getErr: hrisrepo.ErrEmployeeNotFound}
	if w := updateEmployeeThrough(t, repo, true, "gita.permatasari@kantor.local"); w.Code == http.StatusOK || repo.updates != 0 {
		t.Fatalf("MCP update with unreadable record: %d, %d updates", w.Code, repo.updates)
	}

	// The web app may still change the e-mail of an employee without an account.
	repo = &stubEmployeesRepo{stored: stored}
	if w := updateEmployeeThrough(t, repo, false, "other.user@kantor.local"); w.Code != http.StatusOK || repo.updates != 1 {
		t.Fatalf("web e-mail change: %d %s, %d updates", w.Code, w.Body.String(), repo.updates)
	}
}
