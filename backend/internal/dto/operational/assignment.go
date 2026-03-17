package operational

type CreateAssignmentRuleRequest struct {
	RuleType   string         `json:"rule_type" validate:"required,oneof=by_department by_skill by_workload"`
	RuleConfig map[string]any `json:"rule_config" validate:"required"`
	Priority   int            `json:"priority" validate:"required,min=1,max=1000"`
	IsActive   *bool          `json:"is_active"`
}

type UpdateAssignmentRuleRequest struct {
	RuleType   string         `json:"rule_type" validate:"required,oneof=by_department by_skill by_workload"`
	RuleConfig map[string]any `json:"rule_config" validate:"required"`
	Priority   int            `json:"priority" validate:"required,min=1,max=1000"`
	IsActive   *bool          `json:"is_active"`
}
