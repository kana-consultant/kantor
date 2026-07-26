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
	"GET /api/v1/hris/salary-safety": {
		Description: "Salary-safety (min-hours compliance) per employee for a period",
		Query: []QueryParam{
			qi("year", "Period year (default current year)."),
			qi("month", "Period month 1-12 (default current month)."),
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
