package files

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
	marketingrepo "github.com/kana-consultant/kantor/backend/internal/repository/marketing"
)

var (
	ErrUnsupportedType = errors.New("file type is not supported")
	ErrFileNotFound    = errors.New("file not found")
)

type ResolvedFile struct {
	Path       string
	Permission string
}

type Service struct {
	uploadsDir         string
	reimbursementsRepo *hrisrepo.ReimbursementsRepository
	campaignsRepo      *marketingrepo.CampaignsRepository
}

func New(uploadsDir string, reimbursementsRepo *hrisrepo.ReimbursementsRepository, campaignsRepo *marketingrepo.CampaignsRepository) *Service {
	return &Service{
		uploadsDir:         uploadsDir,
		reimbursementsRepo: reimbursementsRepo,
		campaignsRepo:      campaignsRepo,
	}
}

func (s *Service) Resolve(ctx context.Context, fileType string, resourceID string, filename string) (ResolvedFile, error) {
	var (
		relativePath string
		permission   string
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
	default:
		return ResolvedFile{}, ErrUnsupportedType
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
		Path:       absolutePath,
		Permission: permission,
	}, nil
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
