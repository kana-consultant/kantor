package app

import (
	"fmt"

	"github.com/go-chi/chi/v5"

	"github.com/kana-consultant/kantor/backend/internal/config"
	adminhandler "github.com/kana-consultant/kantor/backend/internal/handler/admin"
	fileshandler "github.com/kana-consultant/kantor/backend/internal/handler/files"
	hrishandler "github.com/kana-consultant/kantor/backend/internal/handler/hris"
	marketinghandler "github.com/kana-consultant/kantor/backend/internal/handler/marketing"
	notificationshandler "github.com/kana-consultant/kantor/backend/internal/handler/notifications"
	operationalhandler "github.com/kana-consultant/kantor/backend/internal/handler/operational"
	wahandler "github.com/kana-consultant/kantor/backend/internal/handler/whatsapp"
	"github.com/kana-consultant/kantor/backend/internal/metrics"
)

// BuildRouteTableForInspection builds the real HTTP router — the exact same
// buildRouter used by New — but with inert (nil) services and no database,
// so tests can walk the live route table (e.g. the MCP catalog hardening
// test). The returned router must never serve traffic: every handler would
// dereference a nil service.
//
// Because it calls buildRouter with every handler, adding a new handler
// parameter there fails to compile until it is added here too, which keeps
// the inspected route table complete.
func BuildRouteTableForInspection(cfg config.Config) (chi.Routes, error) {
	application := &App{
		cfg:     cfg,
		metrics: metrics.NewRegistry(),
	}

	handler, err := application.buildRouter(
		nil, // audit service
		nil, // auth service
		nil, // document mail service
		nil, // company profile service
		nil, // PAT service
		nil, // OAuth service
		adminhandler.NewAuditLogsHandler(nil),
		operationalhandler.NewOverviewHandler(nil),
		operationalhandler.NewProjectsHandler(nil, nil, nil, nil),
		operationalhandler.NewKanbanHandler(nil),
		operationalhandler.NewTrackerHandler(nil),
		operationalhandler.NewTrackerReminderHandler(nil),
		operationalhandler.NewDiscordReminderHandler(nil),
		operationalhandler.NewVPSHandler(nil),
		operationalhandler.NewDomainHandler(nil),
		hrishandler.NewOverviewHandler(nil),
		hrishandler.NewEmployeesHandler(nil, nil, cfg.UploadsDir, nil),
		hrishandler.NewHRProfilesHandler(nil),
		hrishandler.NewPayslipsHandler(nil),
		hrishandler.NewContractsHandler(nil),
		hrishandler.NewEmailDeliveriesHandler(nil),
		hrishandler.NewDepartmentsHandler(nil),
		hrishandler.NewCompensationHandler(nil),
		hrishandler.NewCompensationPolicyHandler(nil),
		hrishandler.NewFinanceHandler(nil, nil),
		hrishandler.NewReimbursementsHandler(nil, cfg.UploadsDir, nil),
		hrishandler.NewSubscriptionsHandler(nil, nil),
		marketinghandler.NewOverviewHandler(nil),
		marketinghandler.NewCampaignsHandler(nil, cfg.UploadsDir, nil),
		marketinghandler.NewAdsMetricsHandler(nil, nil),
		marketinghandler.NewLeadsHandler(nil, nil),
		notificationshandler.New(nil),
		fileshandler.New(nil),
		wahandler.New(nil),
	)
	if err != nil {
		return nil, err
	}

	routes, ok := handler.(chi.Routes)
	if !ok {
		return nil, fmt.Errorf("router %T does not expose its routes", handler)
	}
	return routes, nil
}
