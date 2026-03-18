package operational

import (
	"context"
	"errors"
	"sort"
	"strings"

	operationaldto "github.com/kana-consultant/kantor/backend/internal/dto/operational"
	"github.com/kana-consultant/kantor/backend/internal/model"
	operationalrepo "github.com/kana-consultant/kantor/backend/internal/repository/operational"
)

var (
	ErrAssignmentRuleNotFound      = errors.New("assignment rule not found")
	ErrInvalidAssignmentRuleConfig = errors.New("assignment rule config is invalid")
	ErrAutoAssignNoMatch           = errors.New("no project member matched the active assignment rules")
)

type assignmentRulesRepository interface {
	CreateRule(ctx context.Context, projectID string, params operationalrepo.CreateAssignmentRuleParams) (model.AssignmentRule, error)
	ListRules(ctx context.Context, projectID string) ([]model.AssignmentRule, error)
	UpdateRule(ctx context.Context, projectID string, ruleID string, params operationalrepo.UpdateAssignmentRuleParams) (model.AssignmentRule, error)
	DeleteRule(ctx context.Context, projectID string, ruleID string) error
	ListCandidates(ctx context.Context, projectID string) ([]model.AssignmentCandidate, error)
	AutoAssignTask(ctx context.Context, projectID string, taskID string, params operationalrepo.AutoAssignTaskParams) (model.KanbanTask, error)
}

type AssignmentRulesService struct {
	repo assignmentRulesRepository
}

type AutoAssignResult struct {
	Task        model.KanbanTask          `json:"task"`
	MatchedRule model.AssignmentRule      `json:"matched_rule"`
	AssignedTo  model.AssignmentCandidate `json:"assigned_to"`
}

func NewAssignmentRulesService(repo assignmentRulesRepository) *AssignmentRulesService {
	return &AssignmentRulesService{repo: repo}
}

func (s *AssignmentRulesService) CreateRule(ctx context.Context, projectID string, request operationaldto.CreateAssignmentRuleRequest, createdBy string) (model.AssignmentRule, error) {
	config, err := validateRuleConfig(request.RuleType, request.RuleConfig)
	if err != nil {
		return model.AssignmentRule{}, err
	}

	return s.repo.CreateRule(ctx, projectID, operationalrepo.CreateAssignmentRuleParams{
		RuleType:   request.RuleType,
		RuleConfig: config,
		Priority:   request.Priority,
		IsActive:   boolValueOrDefault(request.IsActive, true),
		CreatedBy:  createdBy,
	})
}

func (s *AssignmentRulesService) ListRules(ctx context.Context, projectID string) ([]model.AssignmentRule, error) {
	return s.repo.ListRules(ctx, projectID)
}

func (s *AssignmentRulesService) UpdateRule(ctx context.Context, projectID string, ruleID string, request operationaldto.UpdateAssignmentRuleRequest) (model.AssignmentRule, error) {
	config, err := validateRuleConfig(request.RuleType, request.RuleConfig)
	if err != nil {
		return model.AssignmentRule{}, err
	}

	rule, err := s.repo.UpdateRule(ctx, projectID, ruleID, operationalrepo.UpdateAssignmentRuleParams{
		RuleType:   request.RuleType,
		RuleConfig: config,
		Priority:   request.Priority,
		IsActive:   boolValueOrDefault(request.IsActive, true),
	})
	if errors.Is(err, operationalrepo.ErrAssignmentRuleNotFound) {
		return model.AssignmentRule{}, ErrAssignmentRuleNotFound
	}

	return rule, err
}

func (s *AssignmentRulesService) DeleteRule(ctx context.Context, projectID string, ruleID string) error {
	err := s.repo.DeleteRule(ctx, projectID, ruleID)
	if errors.Is(err, operationalrepo.ErrAssignmentRuleNotFound) {
		return ErrAssignmentRuleNotFound
	}
	return err
}

func (s *AssignmentRulesService) AutoAssignTask(ctx context.Context, projectID string, taskID string, actorUserID string, ipAddress string) (AutoAssignResult, error) {
	rules, err := s.repo.ListRules(ctx, projectID)
	if err != nil {
		return AutoAssignResult{}, err
	}

	candidates, err := s.repo.ListCandidates(ctx, projectID)
	if err != nil {
		return AutoAssignResult{}, err
	}

	for _, rule := range rules {
		if !rule.IsActive {
			continue
		}

		matches := matchCandidates(rule, candidates)
		if len(matches) == 0 {
			continue
		}

		selected := matches[0]
		if rule.RuleType == "by_workload" {
			sort.SliceStable(matches, func(i int, j int) bool {
				if matches[i].Workload == matches[j].Workload {
					if matches[i].AssignedAt.Equal(matches[j].AssignedAt) {
						return matches[i].UserID < matches[j].UserID
					}
					return matches[i].AssignedAt.Before(matches[j].AssignedAt)
				}
				return matches[i].Workload < matches[j].Workload
			})
			selected = matches[0]
		}

		task, err := s.repo.AutoAssignTask(ctx, projectID, taskID, operationalrepo.AutoAssignTaskParams{
			AssigneeID:  selected.UserID,
			ActorUserID: actorUserID,
			IPAddress:   ipAddress,
			Rule:        rule,
		})
		if err != nil {
			return AutoAssignResult{}, err
		}

		return AutoAssignResult{
			Task:        task,
			MatchedRule: rule,
			AssignedTo:  selected,
		}, nil
	}

	return AutoAssignResult{}, ErrAutoAssignNoMatch
}

func validateRuleConfig(ruleType string, config map[string]any) (map[string]any, error) {
	normalized := make(map[string]any, len(config))
	for key, value := range config {
		normalized[key] = value
	}

	switch ruleType {
	case "by_department":
		department := strings.TrimSpace(stringConfigValue(config, "department"))
		if department == "" {
			return nil, ErrInvalidAssignmentRuleConfig
		}
		normalized["department"] = department
	case "by_skill":
		skill := strings.TrimSpace(stringConfigValue(config, "skill"))
		if skill == "" {
			return nil, ErrInvalidAssignmentRuleConfig
		}
		normalized["skill"] = skill
	case "by_workload":
	default:
		return nil, ErrInvalidAssignmentRuleConfig
	}

	if roleInProject := strings.TrimSpace(stringConfigValue(config, "role_in_project")); roleInProject != "" {
		normalized["role_in_project"] = roleInProject
	}

	return normalized, nil
}

func matchCandidates(rule model.AssignmentRule, candidates []model.AssignmentCandidate) []model.AssignmentCandidate {
	matches := make([]model.AssignmentCandidate, 0)
	requiredRole := strings.TrimSpace(stringConfigValue(rule.RuleConfig, "role_in_project"))
	requiredDepartment := strings.TrimSpace(stringConfigValue(rule.RuleConfig, "department"))
	requiredSkill := strings.TrimSpace(stringConfigValue(rule.RuleConfig, "skill"))

	for _, candidate := range candidates {
		if requiredRole != "" && !strings.EqualFold(candidate.RoleInProject, requiredRole) {
			continue
		}

		switch rule.RuleType {
		case "by_department":
			if candidate.Department == nil || !strings.EqualFold(strings.TrimSpace(*candidate.Department), requiredDepartment) {
				continue
			}
		case "by_skill":
			if !candidateHasSkill(candidate, requiredSkill) {
				continue
			}
		case "by_workload":
		default:
			continue
		}

		matches = append(matches, candidate)
	}

	return matches
}

func candidateHasSkill(candidate model.AssignmentCandidate, requiredSkill string) bool {
	for _, skill := range candidate.Skills {
		if strings.EqualFold(strings.TrimSpace(skill), requiredSkill) {
			return true
		}
	}
	return false
}

func stringConfigValue(config map[string]any, key string) string {
	value, ok := config[key]
	if !ok || value == nil {
		return ""
	}

	stringValue, ok := value.(string)
	if !ok {
		return ""
	}

	return stringValue
}

func boolValueOrDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}
