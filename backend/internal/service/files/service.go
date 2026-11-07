package files

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kana-consultant/kantor/backend/internal/model"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
	marketingrepo "github.com/kana-consultant/kantor/backend/internal/repository/marketing"
	"github.com/kana-consultant/kantor/backend/internal/storage"
)

var (
	ErrUnsupportedType = errors.New("file type is not supported")
	ErrFileNotFound    = errors.New("file not found")
)

type ResolvedFile struct {
	Path        string
	StorageKey  string
	Permission  string
	OwnerUserID *string
}

type filesReimbursementsRepository interface {
	FindAttachmentPath(ctx context.Context, reimbursementID string, filename string) (string, error)
}

type filesCampaignsRepository interface {
	FindAttachmentPath(ctx context.Context, campaignID string, filename string) (string, error)
}

type filesEmployeesRepository interface {
	FindAvatarPath(ctx context.Context, employeeID string, filename string) (string, error)
	GetEmployeeByID(ctx context.Context, employeeID string) (model.Employee, error)
}

type Service struct {
	uploadsDir         string
	storage            storage.Storage
	reimbursementsRepo filesReimbursementsRepository
	campaignsRepo      filesCampaignsRepository
	employeesRepo      filesEmployeesRepository
}

func New(
	uploadsDir string,
	reimbursementsRepo filesReimbursementsRepository,
	campaignsRepo filesCampaignsRepository,
	employeesRepo filesEmployeesRepository,
) *Service {
	return &Service{
		uploadsDir:         uploadsDir,
		reimbursementsRepo: reimbursementsRepo,
		campaignsRepo:      campaignsRepo,
		employeesRepo:      employeesRepo,
	}
}

// NewWithStorage creates a files Service using a storage.Storage provider.
func NewWithStorage(
	st storage.Storage,
	uploadsDir string,
	reimbursementsRepo filesReimbursementsRepository,
	campaignsRepo filesCampaignsRepository,
	employeesRepo filesEmployeesRepository,
) *Service {
	return &Service{
		uploadsDir:         uploadsDir,
		storage:            st,
		reimbursementsRepo: reimbursementsRepo,
		campaignsRepo:      campaignsRepo,
		employeesRepo:      employeesRepo,
	}
}

func (s *Service) Resolve(ctx context.Context, fileType string, resourceID string, filename string) (ResolvedFile, error) {
	var (
		relativePath string
		permission   string
		ownerUserID  *string
		err          error
	)

	switch strings.ToLower(strings.TrimSpace(fileType)) {
	case "reimbursements":
		permission = "hris:reimbursement:view"
		relativePath, err = s.reimbursementsRepo.FindAttachmentPath(ctx, resourceID, filename)
		if err != nil {
			if errors.Is(err, hrisrepo.ErrReimbursementNotFound) || errors.Is(err, hrisrepo.ErrReimbursementAttachmentNotFound) {
				return ResolvedFile{}, ErrFileNotFound
			}
			return ResolvedFile{}, err
		}
	case "campaigns":
		permission = "marketing:campaign:view"
		relativePath, err = s.campaignsRepo.FindAttachmentPath(ctx, resourceID, filename)
		if err != nil {
			if errors.Is(err, marketingrepo.ErrCampaignNotFound) || errors.Is(err, marketingrepo.ErrCampaignAttachmentNotFound) {
				return ResolvedFile{}, ErrFileNotFound
			}
			return ResolvedFile{}, err
		}
	case "employees":
		permission = "hris:employee:view"
		relativePath, err = s.employeesRepo.FindAvatarPath(ctx, resourceID, filename)
		if err != nil {
			if errors.Is(err, hrisrepo.ErrEmployeeNotFound) || errors.Is(err, hrisrepo.ErrEmployeeAvatarNotFound) {
				return ResolvedFile{}, ErrFileNotFound
			}
			return ResolvedFile{}, err
		}
		employee, err := s.employeesRepo.GetEmployeeByID(ctx, resourceID)
		if err != nil {
			if errors.Is(err, hrisrepo.ErrEmployeeNotFound) {
				return ResolvedFile{}, ErrFileNotFound
			}
			return ResolvedFile{}, err
		}
		ownerUserID = employee.UserID
	case "profiles":
		permission = ""
		relativePath = filepath.ToSlash(filepath.Join("profiles", resourceID, filename))
		trimmedResourceID := strings.TrimSpace(resourceID)
		if trimmedResourceID != "" {
			ownerUserID = &trimmedResourceID
		}
	default:
		return ResolvedFile{}, ErrUnsupportedType
	}

	normRelative := filepath.ToSlash(relativePath)

	if s.storage != nil {
		exists, err := s.storage.Exists(ctx, normRelative)
		if err != nil {
			return ResolvedFile{}, err
		}
		if !exists {
			return ResolvedFile{}, ErrFileNotFound
		}
		return ResolvedFile{
			Path:        normRelative,
			StorageKey:  normRelative,
			Permission:  permission,
			OwnerUserID: ownerUserID,
		}, nil
	}

	absolutePath, err := s.safeJoin(relativePath)
	if err != nil {
		return ResolvedFile{}, err
	}
	if _, err := os.Stat(absolutePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ResolvedFile{}, ErrFileNotFound
		}
		return ResolvedFile{}, err
	}

	return ResolvedFile{
		Path:        absolutePath,
		StorageKey:  normRelative,
		Permission:  permission,
		OwnerUserID: ownerUserID,
	}, nil
}

// Open retrieves an io.ReadCloser and metadata for the resolved file.
func (s *Service) Open(ctx context.Context, file ResolvedFile) (io.ReadCloser, int64, error) {
	if s.storage != nil {
		rc, meta, err := s.storage.Get(ctx, file.StorageKey)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				return nil, 0, ErrFileNotFound
			}
			return nil, 0, err
		}
		return rc, meta.ContentLength, nil
	}

	f, err := os.Open(file.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, 0, ErrFileNotFound
		}
		return nil, 0, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, 0, err
	}
	return f, info.Size(), nil
}

func (s *Service) safeJoin(relativePath string) (string, error) {
	basePath, err := filepath.Abs(s.uploadsDir)
	if err != nil {
		return "", fmt.Errorf("resolve uploads dir: %w", err)
	}

	candidatePath := filepath.Clean(filepath.Join(basePath, filepath.FromSlash(relativePath)))
	prefix := basePath + string(os.PathSeparator)
	if candidatePath != basePath && !strings.HasPrefix(candidatePath, prefix) {
		return "", ErrFileNotFound
	}

	return candidatePath, nil
}
