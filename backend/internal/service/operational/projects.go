package operational

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/dto/operational"
	"github.com/kana-consultant/kantor/backend/internal/model"
	operationalrepo "github.com/kana-consultant/kantor/backend/internal/repository/operational"
)

var (
	ErrProjectNotFound       = errors.New("project not found")
	ErrInvalidProjectMember  = errors.New("role_in_project is required when assigning project member")
	ErrMissingProjectMember  = errors.New("user_id or user_email is required")
	ErrProjectMemberNotFound = errors.New("project member user not found")
)

type projectsRepository interface {
	CreateProject(ctx context.Context, params operationalrepo.CreateProjectParams) (model.Project, error)
	ListProjects(ctx context.Context, params operationalrepo.ListProjectsParams) ([]model.Project, int64, error)
	GetProjectByID(ctx context.Context, projectID string) (model.Project, error)
	UpdateProject(ctx context.Context, projectID string, params operationalrepo.UpdateProjectParams) (model.Project, error)
	DeleteProject(ctx context.Context, projectID string) error
	ListProjectMembers(ctx context.Context, projectID string) ([]model.ProjectMember, error)
	MutateProjectMember(ctx context.Context, projectID string, params operationalrepo.ProjectMemberMutationParams) error
}

type projectsKanbanRepository interface {
	CreateDefaultColumns(ctx context.Context, projectID string) error
}

type ProjectsService struct {
	repo       projectsRepository
	kanbanRepo projectsKanbanRepository
}

type ProjectDetail struct {
	Project model.Project         `json:"project"`
	Members []model.ProjectMember `json:"members"`
}

func NewProjectsService(repo projectsRepository, kanbanRepo projectsKanbanRepository) *ProjectsService {
	return &ProjectsService{
		repo:       repo,
		kanbanRepo: kanbanRepo,
	}
}

func (s *ProjectsService) CreateProject(ctx context.Context, request operational.CreateProjectRequest, createdBy string) (ProjectDetail, error) {
	project, err := s.repo.CreateProject(ctx, operationalrepo.CreateProjectParams{
		Name:        strings.TrimSpace(request.Name),
		Description: normalizeOptionalString(request.Description),
		Deadline:    normalizeOptionalTime(request.Deadline),
		Status:      request.Status,
		Priority:    request.Priority,
		CreatedBy:   createdBy,
	})
	if err != nil {
		return ProjectDetail{}, err
	}

	if s.kanbanRepo != nil {
		if err := s.kanbanRepo.CreateDefaultColumns(ctx, project.ID); err != nil {
			return ProjectDetail{}, err
		}
	}

	return s.GetProject(ctx, project.ID)
}

func (s *ProjectsService) ListProjects(ctx context.Context, query operational.ListProjectsQuery) ([]model.Project, int64, int, int, error) {
	page := query.Page
	if page <= 0 {
		page = 1
	}

	perPage := query.PerPage
	if perPage <= 0 {
		perPage = 10
	}

	projects, total, err := s.repo.ListProjects(ctx, operationalrepo.ListProjectsParams{
		Page:     page,
		PerPage:  perPage,
		Search:   strings.TrimSpace(query.Search),
		Status:   query.Status,
		Priority: query.Priority,
	})
	if err != nil {
		return nil, 0, 0, 0, err
	}

	return projects, total, page, perPage, nil
}

func (s *ProjectsService) GetProject(ctx context.Context, projectID string) (ProjectDetail, error) {
	project, err := s.repo.GetProjectByID(ctx, projectID)
	if err != nil {
		if errors.Is(err, operationalrepo.ErrProjectNotFound) {
			return ProjectDetail{}, ErrProjectNotFound
		}
		if errors.Is(err, operationalrepo.ErrProjectMemberNotFound) {
			return ProjectDetail{}, ErrProjectMemberNotFound
		}

		return ProjectDetail{}, err
	}

	members, err := s.repo.ListProjectMembers(ctx, projectID)
	if err != nil {
		return ProjectDetail{}, err
	}

	return ProjectDetail{
		Project: project,
		Members: members,
	}, nil
}

func (s *ProjectsService) UpdateProject(ctx context.Context, projectID string, request operational.UpdateProjectRequest) (ProjectDetail, error) {
	_, err := s.repo.UpdateProject(ctx, projectID, operationalrepo.UpdateProjectParams{
		Name:        strings.TrimSpace(request.Name),
		Description: normalizeOptionalString(request.Description),
		Deadline:    normalizeOptionalTime(request.Deadline),
		Status:      request.Status,
		Priority:    request.Priority,
	})
	if err != nil {
		if errors.Is(err, operationalrepo.ErrProjectNotFound) {
			return ProjectDetail{}, ErrProjectNotFound
		}

		return ProjectDetail{}, err
	}

	return s.GetProject(ctx, projectID)
}

func (s *ProjectsService) DeleteProject(ctx context.Context, projectID string) error {
	err := s.repo.DeleteProject(ctx, projectID)
	if errors.Is(err, operationalrepo.ErrProjectNotFound) {
		return ErrProjectNotFound
	}

	return err
}

func (s *ProjectsService) MutateProjectMember(ctx context.Context, projectID string, request operational.ProjectMembersMutationRequest) (ProjectDetail, error) {
	if request.Operation == "assign" && strings.TrimSpace(request.RoleInProject) == "" {
		return ProjectDetail{}, ErrInvalidProjectMember
	}

	if strings.TrimSpace(request.UserID) == "" && strings.TrimSpace(request.UserEmail) == "" {
		return ProjectDetail{}, ErrMissingProjectMember
	}

	err := s.repo.MutateProjectMember(ctx, projectID, operationalrepo.ProjectMemberMutationParams{
		Operation:     request.Operation,
		UserID:        request.UserID,
		UserEmail:     strings.ToLower(strings.TrimSpace(request.UserEmail)),
		RoleInProject: strings.TrimSpace(request.RoleInProject),
	})
	if err != nil {
		if errors.Is(err, operationalrepo.ErrProjectNotFound) {
			return ProjectDetail{}, ErrProjectNotFound
		}

		return ProjectDetail{}, err
	}

	return s.GetProject(ctx, projectID)
}

func normalizeOptionalString(value *string) *string {
	if value == nil {
		return nil
	}

	trimmed := strings.TrimSpace(*value)
	return &trimmed
}

func normalizeOptionalTime(value *time.Time) *string {
	if value == nil {
		return nil
	}

	formatted := value.UTC().Format(time.RFC3339)
	return &formatted
}
