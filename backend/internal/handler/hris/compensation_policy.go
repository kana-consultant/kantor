package hris

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	hrisdto "github.com/kana-consultant/kantor/backend/internal/dto/hris"
	"github.com/kana-consultant/kantor/backend/internal/httputil"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/model"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
	"github.com/kana-consultant/kantor/backend/internal/response"
	hrisservice "github.com/kana-consultant/kantor/backend/internal/service/hris"
)

const salaryDateLayout = "2006-01-02"

type CompensationPolicyHandler struct {
	service   *hrisservice.CompensationPolicyService
	validator *validator.Validate
}

func NewCompensationPolicyHandler(service *hrisservice.CompensationPolicyService) *CompensationPolicyHandler {
	return &CompensationPolicyHandler{
		service:   service,
		validator: validator.New(validator.WithRequiredStructEnabled()),
	}
}

func (h *CompensationPolicyHandler) RegisterRoutes(router chi.Router) {
	router.With(platformmiddleware.RequirePermission("hris:compensation_policy:view")).Get("/compensation-policy", h.getPolicy)
	router.With(platformmiddleware.RequirePermission("hris:compensation_policy:manage")).Put("/compensation-policy", h.updatePolicy)
	router.With(platformmiddleware.RequirePermission("hris:salary_safety:view")).Get("/salary-safety", h.getSalarySafety)
}

func (h *CompensationPolicyHandler) getPolicy(w http.ResponseWriter, r *http.Request) {
	policy, err := h.service.GetPolicy(r.Context())
	if err != nil {
		platformmiddleware.LoggerFromContext(r.Context()).Error("compensation policy get failed", "error", err)
		response.WriteInternalError(r.Context(), w, err, "Failed to load compensation policy")
		return
	}
	response.WriteJSON(w, http.StatusOK, toCompensationPolicyResponse(policy), nil)
}

func (h *CompensationPolicyHandler) updatePolicy(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	var req hrisdto.UpdateCompensationPolicyRequest
	if !httputil.DecodeAndValidate(h.validator, w, r, &req) {
		return
	}

	before, _ := h.service.GetPolicy(r.Context())
	policy, err := h.service.UpdatePolicy(r.Context(), hrisrepo.UpdateCompensationPolicyParams{
		MonthlyBaseSalary: req.MonthlyBaseSalary,
		MinHoursPerDay:    req.MinHoursPerDay,
		MinHoursPerMonth:  req.MinHoursPerMonth,
		Timezone:          strings.TrimSpace(req.Timezone),
		UpdatedBy:         principal.UserID,
	})
	if err != nil {
		if errors.Is(err, hrisservice.ErrCompensationPolicyInvalid) {
			response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
			return
		}
		platformmiddleware.LoggerFromContext(r.Context()).Error("compensation policy update failed", "error", err)
		response.WriteInternalError(r.Context(), w, err, "Failed to update compensation policy")
		return
	}

	platformmiddleware.AuditLog(r.Context(), "update", "hris", "compensation_policy", policy.TenantID, before, policy)
	response.WriteJSON(w, http.StatusOK, toCompensationPolicyResponse(policy), nil)
}

func (h *CompensationPolicyHandler) getSalarySafety(w http.ResponseWriter, r *http.Request) {
	// The current period in the policy timezone is the default for a missing
	// (or unparsable) year/month, and is returned in meta so the UI picks its
	// default period and the last selectable month from the server instead of
	// the browser clock. An explicit value, including 0, is passed through and
	// validated by the service.
	currentYear, currentMonth, err := h.service.CurrentPeriod(r.Context())
	if err != nil {
		platformmiddleware.LoggerFromContext(r.Context()).Error("salary safety period lookup failed", "error", err)
		response.WriteInternalError(r.Context(), w, err, "Failed to evaluate salary safety")
		return
	}
	year := parseIntDefault(r.URL.Query().Get("year"), currentYear)
	month := parseIntDefault(r.URL.Query().Get("month"), currentMonth)

	var employeeID *string
	if raw := strings.TrimSpace(r.URL.Query().Get("employee_id")); raw != "" {
		employeeID = &raw
	}

	evaluations, err := h.service.EvaluateSalarySafety(r.Context(), year, month, employeeID)
	if err != nil {
		if errors.Is(err, hrisservice.ErrCompensationPolicyInvalid) {
			response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
			return
		}
		platformmiddleware.LoggerFromContext(r.Context()).Error("salary safety evaluation failed", "error", err)
		response.WriteInternalError(r.Context(), w, err, "Failed to evaluate salary safety")
		return
	}

	out := make([]hrisdto.SalarySafetyResponse, 0, len(evaluations))
	for _, evaluation := range evaluations {
		out = append(out, toSalarySafetyResponse(evaluation))
	}
	response.WriteJSON(w, http.StatusOK, out, hrisdto.SalarySafetyMeta{
		PeriodYear:   year,
		PeriodMonth:  month,
		CurrentYear:  currentYear,
		CurrentMonth: currentMonth,
	})
}

func toCompensationPolicyResponse(policy model.CompensationPolicy) hrisdto.CompensationPolicyResponse {
	return hrisdto.CompensationPolicyResponse{
		MonthlyBaseSalary: policy.MonthlyBaseSalary,
		MinHoursPerDay:    policy.MinHoursPerDay,
		MinHoursPerMonth:  policy.MinHoursPerMonth,
		Timezone:          policy.Timezone,
		UpdatedAt:         policy.UpdatedAt.Format(time.RFC3339),
	}
}

func toSalarySafetyResponse(evaluation model.SalarySafetyEvaluation) hrisdto.SalarySafetyResponse {
	violations := make([]hrisdto.DailyHoursViolationResponse, 0, len(evaluation.DailyViolations))
	for _, violation := range evaluation.DailyViolations {
		violations = append(violations, hrisdto.DailyHoursViolationResponse{
			Date:        violation.Date.Format(salaryDateLayout),
			ActiveHours: violation.ActiveHours,
		})
	}

	var evaluatedThrough *string
	if evaluation.EvaluatedThrough != nil {
		value := evaluation.EvaluatedThrough.Format(salaryDateLayout)
		evaluatedThrough = &value
	}
	var reason *string
	if evaluation.Reason != nil {
		value := string(*evaluation.Reason)
		reason = &value
	}

	return hrisdto.SalarySafetyResponse{
		EmployeeID:          evaluation.EmployeeID,
		UserID:              evaluation.UserID,
		FullName:            evaluation.FullName,
		PeriodYear:          evaluation.PeriodYear,
		PeriodMonth:         evaluation.PeriodMonth,
		MonthlyActiveHours:  evaluation.MonthlyActiveHours,
		MinHoursPerMonth:    evaluation.MinHoursPerMonth,
		MinHoursPerDay:      evaluation.MinHoursPerDay,
		BaseSalary:          evaluation.BaseSalary,
		DailyViolations:     violations,
		Status:              string(evaluation.Status),
		ExpectedHoursToDate: evaluation.ExpectedHoursToDate,
		TargetHoursMonth:    evaluation.TargetHoursMonth,
		WorkingDaysElapsed:  evaluation.WorkingDaysElapsed,
		WorkingDaysTotal:    evaluation.WorkingDaysTotal,
		EvaluatedThrough:    evaluatedThrough,
		HasTrackerData:      evaluation.HasTrackerData,
		ShortDays:           evaluation.ShortDays,
		AbsentDays:          evaluation.AbsentDays,
		Reason:              reason,
		PositionUnset:       evaluation.PositionUnset,
	}
}

func parseIntDefault(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	return value
}
