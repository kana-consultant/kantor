package mcp

// endpointAnnotations enriches data-heavy list/query routes with their real
// filters + pagination. Keyed by "METHOD path"; unlisted routes stay generic.
var endpointAnnotations = map[string]EndpointMeta{
	// ---- HRIS ----
	"GET /api/v1/hris/employees": {
		Description: "List employees with search and filters",
		Paginated:   true, PerPageDefault: 10, PerPageMax: 100,
		Query: []QueryParam{
			qs("search", "Free-text search over name/email."),
			qs("department", "Filter by department name."),
			qe("status", "Employment status filter.", "active", "probation", "resigned", "terminated"),
		},
	},
	"PUT /api/v1/hris/employees/{employeeID}": {
		Description: "Update an employee record. This is a FULL replacement of the editable fields: read the employee first and send every field again. Through MCP the e-mail cannot be changed (send the stored one; change it in the web app). A masked bank_account_number (******1234) keeps the stored number",
		Body:        "{full_name, email, position, date_joined: \"YYYY-MM-DD\", employment_status: \"active\" | \"probation\" | \"resigned\" | \"terminated\", phone?, department?, address?, emergency_contact?, avatar_url?, bank_account_number?, bank_name?, linkedin_profile?, ssh_keys?}",
	},
	"GET /api/v1/hris/salary-safety": {
		Description: "Salary-safety per employee for a month: actual tracked hours (whole month, weekends and today included) vs. the monthly target pro-rated by weekdays elapsed through yesterday (or month end for past months) since max(1st, date_joined). safe = actual >= expected_hours_to_date, at_risk = below; short_days/absent_days are warnings only. no_data with reason no_user, future_month or target_not_applicable (Part Time, Internship, Project Based, Outsourcing). Hours are in 0.01 h (actual rounded down, target rounded up), so monthly_active_hours >= expected_hours_to_date exactly when safe. Employees who have not started yet (date_joined after today) are not listed. meta holds the evaluated period and the current period in the policy timezone.",
		Query: []QueryParam{
			qi("year", "Period year (default current year in the policy timezone)."),
			qi("month", "Period month 1-12 (default current month in the policy timezone)."),
			qs("employee_id", "Restrict to a single employee (UUID)."),
		},
	},
	"GET /api/v1/hris/finance/records": {
		Description: "List finance records with filters",
		Paginated:   true, PerPageDefault: 20, PerPageMax: 100,
		Query: []QueryParam{
			qs("type", "Record type filter (e.g. income/expense)."),
			qs("category", "Category filter."),
			qs("status", "Status filter."),
			qi("month", "Month 1-12."),
			qi("year", "Year."),
		},
	},
	"GET /api/v1/hris/finance/categories": {
		Description: "List finance categories",
		Query:       []QueryParam{qs("type", "Filter by category type.")},
	},
	"GET /api/v1/hris/finance/summary": {
		Description: "Finance summary totals for a year",
		Query:       []QueryParam{qi("year", "Year (default current year).")},
	},
	"GET /api/v1/hris/reimbursements": {
		Description: "List reimbursements with filters and sorting",
		Paginated:   true, PerPageDefault: 20, PerPageMax: 100,
		Query: []QueryParam{
			qs("status", "Status filter."),
			qs("employee", "Filter by employee."),
			qs("sort_by", "Field to sort by."),
			qe("sort_order", "Sort direction.", "asc", "desc"),
			qi("month", "Month 1-12."),
			qi("year", "Year."),
		},
	},
	"GET /api/v1/hris/reimbursements/summary": {
		Description: "Reimbursement summary for a period",
		Query:       []QueryParam{qi("month", "Month 1-12."), qi("year", "Year.")},
	},

	// ---- HRIS documents: payslips ----
	// Every payslip tool needs the matching hris:payslip:* permission AND
	// hris:salary:view. The rendered PDF/DOCX are not tools. Through MCP a
	// document can only be mailed to the login e-mail of the employee's linked
	// account, and the send must name that address (expected_recipient[s]).
	"GET /api/v1/hris/payslips": {
		Description: "Payslips of one month: one row per employee, either a stored slip (status draft or sent) or a live preview that is not stored yet (status none, preview=true). Each row has employee_id, payslip_id, totals, warnings, blocked (cannot be generated, e.g. no salary record), render_status (none/pending/rendering/ready/failed), has_pdf and last_delivery. The response also has summary counts, pdf_available (false = the server cannot make PDFs, render_status then stays none) and document_mail_ready. Returns salary amounts for every employee",
		Query: []QueryParam{
			qi("year", "Period year (required)."),
			qi("month", "Period month 1-12 (required)."),
		},
	},
	"POST /api/v1/hris/payslips/generate": {
		Description: "Create or rebuild DRAFT payslips for a month. No e-mail is sent. Returns generated[] and skipped[] (with the reason); the PDFs render in the background, so poll the payslip detail tool until render_status is ready before sending. Rebuilding a draft without pay_date resets its pay date to the default",
		Body:        "{year: integer, month: 1-12, employee_ids: [employee uuid] (the employee_id values of GET /hris/payslips rows; max 500), pay_date?: \"YYYY-MM-DD\" (default: the company payday moved back to a weekday)}",
	},
	"GET /api/v1/hris/payslips/employee/{employeeID}": {
		Description: "Payslip history of one employee, newest first",
		Query:       []QueryParam{qi("limit", "Maximum rows, 1-120.")},
	},
	"GET /api/v1/hris/payslips/{payslipID}": {
		Description: "One payslip: header (the account number is masked), earnings, deductions, reimbursements, manual lines, totals, warnings, render state and last delivery",
	},
	"PUT /api/v1/hris/payslips/{payslipID}": {
		Description: "Edit a DRAFT payslip's note and manual adjustment lines and re-render it. Both fields REPLACE what is stored, so read the slip first and ALWAYS send both: leaving manual_lines out removes every adjustment line (the amounts change), leaving note out clears the note. A sent slip cannot be edited (use void-reissue)",
		Body:        "{note: string (one line, max 120 characters; \"\" clears it), manual_lines: [{label: string (max 60), amount: integer rupiah (positive = earning, negative = deduction), keterangan?: string (max 60)}] (max 5; [] removes every line)}",
	},
	"GET /api/v1/hris/payslips/{payslipID}/recipient": {
		Description: "Who would receive this payslip e-mail: recipient address, recipient_source and is_new (differs from the last delivery to this employee). Sends nothing. Through MCP only a recipient_source of login can be sent to; anything else has to be sent from the web app",
		Query:       []QueryParam{qe("source", "Recipient source (default: login e-mail, or the employee e-mail when no account is linked).", "default", "login", "employee", "personal")},
	},
	"POST /api/v1/hris/payslips/send-preview": {
		Description: "Preview a batch send: recipients[] (payslip_id, employee_name, recipient) of every slip that would be mailed, skipped[] with the reason (through MCP also every slip whose employee has no linked login account: those must be sent from the web app) and whether document e-mail is ready. Sends nothing. Show the recipients to the human before send-batch",
		Body:        "{ids: [payslip uuid] (max 200), include_already_sent?: boolean (default false: slips already sent are skipped)}",
	},
	"POST /api/v1/hris/payslips/send-batch": {
		Description:    "SENDS E-MAIL and cannot be undone: queues the payslip PDF of every listed slip to the login e-mail of its employee. Before calling: run send-preview, show the human every recipient and get an explicit yes; then pass confirm=true and expected_recipients with exactly the addresses the human approved. The call is refused (nothing is sent) if an address differs from what the server resolves now. Returns queued[] and skipped[]; delivery results appear as last_delivery on each payslip",
		Body:           "{ids: [payslip uuid] (max 200), expected_recipients: {payslip uuid: recipient address from send-preview} (one entry for every slip to be sent), include_already_sent?: boolean (default false)}",
		Destructive:    true,
		OpenWorld:      true,
		RequireConfirm: true,
	},
	"POST /api/v1/hris/payslips/{payslipID}/send": {
		Description:    "SENDS E-MAIL and cannot be undone: sends this payslip PDF to the login e-mail of the employee. Before calling: call the recipient tool, tell the human the address and get an explicit yes; then pass confirm=true and expected_recipient with that address. Refused (nothing is sent) when the employee has no linked login account or the address differs from what the server resolves now. The response says whether it was sent (sent, error_category, error_message)",
		Body:           "{expected_recipient: the address returned by the recipient tool and approved by the human}",
		Destructive:    true,
		OpenWorld:      true,
		RequireConfirm: true,
	},
	"POST /api/v1/hris/payslips/{payslipID}/void-reissue": {
		Description: "Void a SENT payslip and create its replacement draft (same number with -R<n>). The voided slip stays on record and this cannot be undone, so ask the human first. The replacement still has to be sent",
		Body:        "{reason: string (3-200 characters)}",
		Destructive: true,
	},

	// ---- HRIS documents: employment contracts ----
	// hris:contract:view / :manage / :send. Compensation is returned and
	// editable only with hris:salary:view. The rendered files are not tools.
	"GET /api/v1/hris/contracts": {
		Description: "List employment contracts with status (draft, generated, sent, signed, ended, cancelled), document numbers, render state, last delivery and the notice/expired flags",
		Query: []QueryParam{
			qs("employee_id", "Restrict to one employee (UUID)."),
			qe("status", "Contract status filter.", "draft", "generated", "sent", "signed", "ended", "cancelled"),
			qe("type", "Contract type filter.", "PKWT", "PKWTT", "MAGANG"),
			qs("search", "Free-text search."),
			qi("limit", "Maximum rows, 1-500."),
		},
	},
	"POST /api/v1/hris/contracts": {
		Description: "Create a contract DRAFT for an employee. A PKWT later produces two documents (PKWT + NDA/HKI); PKWTT and MAGANG are record-only. Nothing is generated or sent yet",
		Body:        "{employee_id: uuid, contract_type: \"PKWT\" | \"PKWTT\" | \"MAGANG\", is_record_only?: boolean (record only, no documents), start_date: \"YYYY-MM-DD\", end_date?: \"YYYY-MM-DD\" (required for PKWT), job_title: string, department?, supervisor_name?, work_location?, work_mode?: \"wfo\" | \"hybrid\" | \"remote\", work_mode_detail?, pkwt_basis?: one Indonesian sentence, job_description?: one Indonesian sentence (max 600), work_days?, work_hours?, weekly_hours?: integer (default 40), notice_days?: integer (default 30), compensation?: {base_salary, fixed_allowance} in rupiah (needs hris:salary:view; omitted = prefilled from the salary record), benefits?: [{name, value?, notes?}] (max 10), incident_report_hours?, non_solicit_months?, confidentiality_years?, prior_works?: [{title, description?, year?: \"YYYY\"}] (max 10), document_date?: \"YYYY-MM-DD\", document_city?}",
	},
	"GET /api/v1/hris/contracts/{contractID}": {
		Description: "One contract with every term. compensation is present only when the caller has hris:salary:view",
	},
	"PUT /api/v1/hris/contracts/{contractID}": {
		Description: "Edit a contract that is a draft, generated, or sent but not signed. This is a FULL replacement: read the contract first and send every field again (an omitted compensation, document_date or document_city keeps the stored value; any other omitted field is cleared or reset to its default). A generated or sent contract goes back to draft (a sent one with revision+1, same numbers) and must be generated again before it can be sent; a signed contract cannot be edited (use renew)",
		Body:        "{contract_type: \"PKWT\" | \"PKWTT\" | \"MAGANG\", is_record_only?, start_date: \"YYYY-MM-DD\", end_date?: \"YYYY-MM-DD\", job_title, department?, supervisor_name?, work_location?, work_mode?: \"wfo\" | \"hybrid\" | \"remote\", work_mode_detail?, pkwt_basis?, job_description?, work_days?, work_hours?, weekly_hours?, notice_days?, compensation?: {base_salary, fixed_allowance}, benefits?: [{name, value?, notes?}], incident_report_hours?, non_solicit_months?, confidentiality_years?, prior_works?: [{title, description?, year?}], document_date?, document_city?}",
	},
	"GET /api/v1/hris/contracts/{contractID}/preflight": {
		Description: "What a contract still needs before its documents can be generated: ready, missing[] (scope company, identity, employee or contract) and warnings[]. Company profile and employee identity data can only be completed in the web app",
	},
	"POST /api/v1/hris/contracts/{contractID}/generate": {
		Description: "Generate the documents of a contract (PKWT + NDA/HKI). The first generation assigns the official document numbers, which then stay fixed; the PDFs render in the background, so poll the contract until render_status is ready. Fails listing the missing fields when preflight is not ready. No e-mail is sent",
		Body:        "{} (no fields)",
	},
	"POST /api/v1/hris/contracts/{contractID}/renew": {
		Description: "Create a renewal DRAFT that continues a signed or ended contract with an end date; it starts the day after that end date",
		Body:        "{} (no fields)",
	},
	"PATCH /api/v1/hris/contracts/{contractID}/status": {
		Description: "Mark a contract signed, ended or cancelled. This cannot be undone, so ask the human first",
		Body:        "{status: \"signed\" | \"ended\" | \"cancelled\", signed_at?: \"YYYY-MM-DD\" (default today, not in the future), ended_at?: \"YYYY-MM-DD\" (default today, not in the future), end_notes?: string (max 500)}",
		Destructive: true,
	},
	"GET /api/v1/hris/contracts/{contractID}/recipient": {
		Description: "Who would receive this contract e-mail: recipient address, recipient_source and is_new. Sends nothing. Through MCP only a recipient_source of login can be sent to, and without cc; anything else has to be sent from the web app",
		Query:       []QueryParam{qe("source", "Recipient source (default: login e-mail, or the employee e-mail when no account is linked).", "default", "login", "employee", "personal")},
	},
	"POST /api/v1/hris/contracts/{contractID}/send": {
		Description:    "SENDS E-MAIL and cannot be undone: sends the PKWT and the NDA/HKI PDFs together to the login e-mail of the employee. Before calling: call the recipient tool, tell the human the address and get an explicit yes; then pass confirm=true and expected_recipient with that address. Refused (nothing is sent) when the employee has no linked login account, when a cc is given (cc is web-only) or when the address differs from what the server resolves now. The response says whether it was sent (sent, error_category, error_message)",
		Body:           "{expected_recipient: the address returned by the recipient tool and approved by the human}",
		Destructive:    true,
		OpenWorld:      true,
		RequireConfirm: true,
	},

	// ---- Marketing ----
	"GET /api/v1/marketing/campaigns": {
		Description: "List marketing campaigns with filters",
		Paginated:   true, PerPageDefault: 12, PerPageMax: 100,
		Query: []QueryParam{
			qs("search", "Free-text search."),
			qs("channel", "Channel filter."),
			qs("status", "Status filter."),
			qs("pic", "Person-in-charge filter."),
			qs("date_from", "On/after this date (YYYY-MM-DD)."),
			qs("date_to", "On/before this date (YYYY-MM-DD)."),
		},
	},
	"GET /api/v1/marketing/ads-metrics": {
		Description: "List ads metrics with filters",
		Paginated:   true, PerPageDefault: 20, PerPageMax: 100,
		Query: []QueryParam{
			qs("campaign_id", "Filter by campaign (UUID)."),
			qs("platform", "Ad platform filter."),
			qs("date_from", "On/after this date (YYYY-MM-DD)."),
			qs("date_to", "On/before this date (YYYY-MM-DD)."),
		},
	},
	"GET /api/v1/marketing/ads-metrics/summary": {
		Description: "Aggregated ads metrics summary",
		Query: []QueryParam{
			qs("group_by", "Grouping dimension."),
			qs("date_from", "On/after this date (YYYY-MM-DD)."),
			qs("date_to", "On/before this date (YYYY-MM-DD)."),
		},
	},
	"GET /api/v1/marketing/leads": {
		Description: "List leads with pipeline filters",
		Paginated:   true, PerPageDefault: 20, PerPageMax: 100,
		Query: []QueryParam{
			qs("pipeline_status", "Pipeline stage filter."),
			qs("source_channel", "Acquisition channel filter."),
			qs("campaign_id", "Filter by campaign (UUID)."),
			qs("assigned_to", "Filter by assignee."),
			qs("search", "Free-text search."),
			qs("date_from", "On/after this date (YYYY-MM-DD)."),
			qs("date_to", "On/before this date (YYYY-MM-DD)."),
		},
	},

	// ---- Operational ----
	"GET /api/v1/operational/projects": {
		Description: "List projects with filters",
		Paginated:   true, PerPageDefault: 10, PerPageMax: 100,
		Query: []QueryParam{
			qs("search", "Free-text search."),
			qs("status", "Status filter."),
			qs("priority", "Priority filter."),
		},
	},
	"GET /api/v1/operational/vps": {
		Description: "List VPS inventory (returns all matching rows)",
		Query: []QueryParam{
			qs("status", "Status filter."),
			qs("provider", "Provider filter."),
			qs("tag", "Tag filter."),
			qs("search", "Free-text search."),
		},
	},
	"GET /api/v1/operational/domains": {
		Description: "List domain inventory (returns all matching rows)",
		Query: []QueryParam{
			qs("status", "Status filter."),
			qs("registrar", "Registrar filter."),
			qs("tag", "Tag filter."),
			qs("search", "Free-text search."),
		},
	},
	"GET /api/v1/operational/tasks/mine": {
		Description: "Your assigned tasks across all projects (status: open/done/overdue). Self-scoped to the caller.",
		Query:       []QueryParam{qe("status", "Computed status filter.", "open", "done", "overdue", "all")},
	},

	// ---- Tracker ----
	"GET /api/v1/tracker/my-activity": {
		Description: "Your activity-tracker overview for a date range",
		Query:       []QueryParam{qs("date_from", "Start date (YYYY-MM-DD)."), qs("date_to", "End date (YYYY-MM-DD).")},
	},
	"GET /api/v1/tracker/team-activity": {
		Description: "Team activity-tracker overview for a date range",
		Query: []QueryParam{
			qs("date_from", "Start date (YYYY-MM-DD)."),
			qs("date_to", "End date (YYYY-MM-DD)."),
			qs("user_id", "Restrict to a single user (UUID)."),
		},
	},
	"GET /api/v1/tracker/activity/{userID}": {
		Description: "A specific user's activity-tracker overview",
		Query:       []QueryParam{qs("date_from", "Start date (YYYY-MM-DD)."), qs("date_to", "End date (YYYY-MM-DD).")},
	},
	"GET /api/v1/tracker/summary": {
		Description: "Daily tracker summary",
		Query:       []QueryParam{qs("date", "Day (YYYY-MM-DD, default today).")},
	},

	// ---- WhatsApp ----
	"GET /api/v1/wa/templates": {
		Description: "List WA message templates",
		Query:       []QueryParam{qs("category", "Category filter."), qs("trigger_type", "Trigger type filter.")},
	},
	"GET /api/v1/wa/logs": {
		Description: "List WA broadcast logs",
		Paginated:   true, PerPageDefault: 20,
		Query: []QueryParam{
			qs("schedule_id", "Filter by schedule (UUID)."),
			qs("trigger_type", "Trigger type filter."),
			qs("template_slug", "Template slug filter."),
			qs("status", "Delivery status filter."),
			qs("search", "Free-text search."),
			qs("date_from", "On/after this date (YYYY-MM-DD)."),
			qs("date_to", "On/before this date (YYYY-MM-DD)."),
		},
	},

	// ---- Notifications ----
	"GET /api/v1/notifications": {
		Description: "List your notifications",
		Paginated:   true, PerPageDefault: 20, PerPageMax: 100,
		Query: []QueryParam{qb("read", "Filter by read/unread state.")},
	},

	// ---- Admin ----
	"GET /api/v1/admin/audit-logs": {
		Description: "Search the audit log",
		Paginated:   true, PerPageDefault: 20,
		Query: []QueryParam{
			qs("module", "Module filter."),
			qs("action", "Action filter."),
			qs("user_id", "Actor user (UUID)."),
			qs("resource", "Resource type filter."),
			qs("resource_id", "Resource id filter."),
			qs("search", "Free-text search."),
			qs("date_from", "On/after this date (YYYY-MM-DD)."),
			qs("date_to", "On/before this date (YYYY-MM-DD)."),
		},
	},
	"GET /api/v1/admin/audit-logs/users": {
		Description: "List users that appear in the audit log",
		Query:       []QueryParam{qs("search", "Free-text search.")},
	},
	"GET /api/v1/admin/roles": {
		Description: "List roles (returns the full filtered list)",
		Query: []QueryParam{
			qs("search", "Free-text search over name/slug."),
			qb("is_system", "Filter system vs custom roles."),
			qb("is_active", "Filter active/inactive roles."),
		},
	},
	"GET /api/v1/admin/users": {
		Description: "List users with filters",
		Paginated:   true, PerPageDefault: 20,
		Query: []QueryParam{
			qs("search", "Free-text search over name/email."),
			qs("module", "Filter by assigned module."),
			qs("role", "Filter by assigned role (UUID)."),
			qb("super_admin", "Filter super admins."),
		},
	},
}

// annotationFor returns the curated metadata for a route, if any.
func annotationFor(method string, route string) *EndpointMeta {
	if meta, ok := endpointAnnotations[method+" "+route]; ok {
		copy := meta
		return &copy
	}
	return nil
}

func qs(name, desc string) QueryParam {
	return QueryParam{Name: name, Type: "string", Description: desc}
}
func qi(name, desc string) QueryParam {
	return QueryParam{Name: name, Type: "integer", Description: desc}
}
func qb(name, desc string) QueryParam {
	return QueryParam{Name: name, Type: "boolean", Description: desc}
}
func qe(name, desc string, enum ...string) QueryParam {
	return QueryParam{Name: name, Type: "string", Description: desc, Enum: enum}
}
