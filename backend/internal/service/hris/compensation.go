package hris

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	hrisdto "github.com/kana-consultant/kantor/backend/internal/dto/hris"
	"github.com/kana-consultant/kantor/backend/internal/model"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
	"github.com/kana-consultant/kantor/backend/internal/security"
)

var (
	ErrSalaryNotFound  = errors.New("salary record not found")
	ErrBonusNotFound   = errors.New("bonus record not found")
	ErrBonusNotPending = errors.New("only pending bonus records can be changed")
)

type compensationRepository interface {
	CreateSalary(ctx context.Context, params hrisrepo.CreateSalaryParams) (hrisrepo.SalaryRow, error)
	ListSalaries(ctx context.Context, employeeID string) ([]hrisrepo.SalaryRow, error)
	GetCurrentSalary(ctx context.Context, employeeID string) (hrisrepo.SalaryRow, error)
	LogSalaryAccess(ctx context.Context, userID string, employeeID string, action string) error
	CreateBonus(ctx context.Context, params hrisrepo.CreateBonusParams) (hrisrepo.BonusRow, error)
	ListBonuses(ctx context.Context, employeeID string) ([]hrisrepo.BonusRow, error)
	GetBonusByID(ctx context.Context, bonusID string) (hrisrepo.BonusRow, error)
	UpdateBonus(ctx context.Context, bonusID string, params hrisrepo.UpdateBonusParams) (hrisrepo.BonusRow, error)
	UpdateBonusApprovalStatus(ctx context.Context, bonusID string, status string, approverID string) (hrisrepo.BonusRow, error)
	DeleteBonus(ctx context.Context, bonusID string) error
}

type compensationEmployeesRepository interface {
	GetEmployeeByID(ctx context.Context, employeeID string) (model.Employee, error)
}

type CompensationService struct {
	repo          compensationRepository
	employeesRepo compensationEmployeesRepository
	encrypter     *security.Encrypter
}

func NewCompensationService(repo compensationRepository, employeesRepo compensationEmployeesRepository, encrypter *security.Encrypter) *CompensationService {
	return &CompensationService{
		repo:          repo,
		employeesRepo: employeesRepo,
		encrypter:     encrypter,
	}
}

func (s *CompensationService) CreateSalary(ctx context.Context, employeeID string, request hrisdto.CreateSalaryRequest, actorID string) (model.SalaryRecord, error) {
	if _, err := s.employeesRepo.GetEmployeeByID(ctx, employeeID); err != nil {
		return model.SalaryRecord{}, err
	}

	netSalary := request.BaseSalary + sumAmountMap(request.Allowances) - sumAmountMap(request.Deductions)
	baseCipher, err := s.encrypter.EncryptString(strconv.FormatInt(request.BaseSalary, 10))
	if err != nil {
		return model.SalaryRecord{}, err
	}
	allowancesCipher, err := encryptAmountMap(s.encrypter, request.Allowances)
	if err != nil {
		return model.SalaryRecord{}, err
	}
	deductionsCipher, err := encryptAmountMap(s.encrypter, request.Deductions)
	if err != nil {
		return model.SalaryRecord{}, err
	}
	netCipher, err := s.encrypter.EncryptString(strconv.FormatInt(netSalary, 10))
	if err != nil {
		return model.SalaryRecord{}, err
	}

	row, err := s.repo.CreateSalary(ctx, hrisrepo.CreateSalaryParams{
		EmployeeID:    employeeID,
		BaseSalary:    baseCipher,
		Allowances:    allowancesCipher,
		Deductions:    deductionsCipher,
		NetSalary:     netCipher,
		EffectiveDate: request.EffectiveDate,
		CreatedBy:     actorID,
	})
	if err != nil {
		return model.SalaryRecord{}, err
	}

	return s.mapSalaryRow(row)
}

func (s *CompensationService) ListSalaries(ctx context.Context, employeeID string, actorID string) ([]model.SalaryRecord, error) {
	rows, err := s.repo.ListSalaries(ctx, employeeID)
	if err != nil {
		return nil, err
	}
	if err := s.repo.LogSalaryAccess(ctx, actorID, employeeID, "salary_history_view"); err != nil {
		return nil, err
	}

	result := make([]model.SalaryRecord, 0, len(rows))
	for _, row := range rows {
		record, err := s.mapSalaryRow(row)
		if err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, nil
}

func (s *CompensationService) GetCurrentSalary(ctx context.Context, employeeID string, actorID string) (model.SalaryRecord, error) {
	row, err := s.repo.GetCurrentSalary(ctx, employeeID)
	if err != nil {
		if errors.Is(err, hrisrepo.ErrSalaryNotFound) {
			return model.SalaryRecord{}, ErrSalaryNotFound
		}
		return model.SalaryRecord{}, err
	}
	if err := s.repo.LogSalaryAccess(ctx, actorID, employeeID, "salary_current_view"); err != nil {
		return model.SalaryRecord{}, err
	}
	return s.mapSalaryRow(row)
}

func (s *CompensationService) CreateBonus(ctx context.Context, employeeID string, request hrisdto.CreateBonusRequest, actorID string) (model.BonusRecord, error) {
	if _, err := s.employeesRepo.GetEmployeeByID(ctx, employeeID); err != nil {
		return model.BonusRecord{}, err
	}

	amountCipher, err := s.encrypter.EncryptString(strconv.FormatInt(request.Amount, 10))
	if err != nil {
		return model.BonusRecord{}, err
	}

	row, err := s.repo.CreateBonus(ctx, hrisrepo.CreateBonusParams{
		EmployeeID:  employeeID,
		Amount:      amountCipher,
		Reason:      strings.TrimSpace(request.Reason),
		PeriodMonth: request.PeriodMonth,
		PeriodYear:  request.PeriodYear,
		CreatedBy:   actorID,
	})
	if err != nil {
		return model.BonusRecord{}, err
	}
	return s.mapBonusRow(row)
}

func (s *CompensationService) ListBonuses(ctx context.Context, employeeID string) ([]model.BonusRecord, error) {
	rows, err := s.repo.ListBonuses(ctx, employeeID)
	if err != nil {
		return nil, err
	}
	result := make([]model.BonusRecord, 0, len(rows))
	for _, row := range rows {
		record, err := s.mapBonusRow(row)
		if err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, nil
}

func (s *CompensationService) UpdateBonus(ctx context.Context, bonusID string, request hrisdto.UpdateBonusRequest) (model.BonusRecord, error) {
	row, err := s.repo.GetBonusByID(ctx, bonusID)
	if err != nil {
		if errors.Is(err, hrisrepo.ErrBonusNotFound) {
			return model.BonusRecord{}, ErrBonusNotFound
		}
		return model.BonusRecord{}, err
	}
	if row.ApprovalStatus != "pending" {
		return model.BonusRecord{}, ErrBonusNotPending
	}

	amountCipher, err := s.encrypter.EncryptString(strconv.FormatInt(request.Amount, 10))
	if err != nil {
		return model.BonusRecord{}, err
	}

	row, err = s.repo.UpdateBonus(ctx, bonusID, hrisrepo.UpdateBonusParams{
		Amount:      amountCipher,
		Reason:      strings.TrimSpace(request.Reason),
		PeriodMonth: request.PeriodMonth,
		PeriodYear:  request.PeriodYear,
	})
	if err != nil {
		if errors.Is(err, hrisrepo.ErrBonusNotFound) {
			return model.BonusRecord{}, ErrBonusNotFound
		}
		return model.BonusRecord{}, err
	}

	return s.mapBonusRow(row)
}

func (s *CompensationService) ApproveBonus(ctx context.Context, bonusID string, actorID string) (model.BonusRecord, error) {
	row, err := s.repo.UpdateBonusApprovalStatus(ctx, bonusID, "approved", actorID)
	if err != nil {
		if errors.Is(err, hrisrepo.ErrBonusNotFound) {
			return model.BonusRecord{}, ErrBonusNotFound
		}
		return model.BonusRecord{}, err
	}
	return s.mapBonusRow(row)
}

func (s *CompensationService) RejectBonus(ctx context.Context, bonusID string, actorID string) (model.BonusRecord, error) {
	row, err := s.repo.UpdateBonusApprovalStatus(ctx, bonusID, "rejected", actorID)
	if err != nil {
		if errors.Is(err, hrisrepo.ErrBonusNotFound) {
			return model.BonusRecord{}, ErrBonusNotFound
		}
		return model.BonusRecord{}, err
	}
	return s.mapBonusRow(row)
}

func (s *CompensationService) DeleteBonus(ctx context.Context, bonusID string) error {
	row, err := s.repo.GetBonusByID(ctx, bonusID)
	if err != nil {
		if errors.Is(err, hrisrepo.ErrBonusNotFound) {
			return ErrBonusNotFound
		}
		return err
	}
	if row.ApprovalStatus != "pending" {
		return ErrBonusNotPending
	}

	if err := s.repo.DeleteBonus(ctx, bonusID); err != nil {
		if errors.Is(err, hrisrepo.ErrBonusNotFound) {
			return ErrBonusNotFound
		}
		return err
	}
	return nil
}

func (s *CompensationService) mapSalaryRow(row hrisrepo.SalaryRow) (model.SalaryRecord, error) {
	baseSalary, err := decryptAmount(s.encrypter, row.BaseSalary)
	if err != nil {
		return model.SalaryRecord{}, err
	}
	allowances, err := decryptAmountMap(s.encrypter, row.Allowances)
	if err != nil {
		return model.SalaryRecord{}, err
	}
	deductions, err := decryptAmountMap(s.encrypter, row.Deductions)
	if err != nil {
		return model.SalaryRecord{}, err
	}
	netSalary, err := decryptAmount(s.encrypter, row.NetSalary)
	if err != nil {
		return model.SalaryRecord{}, err
	}

	return model.SalaryRecord{
		ID:            row.ID,
		EmployeeID:    row.EmployeeID,
		BaseSalary:    baseSalary,
		Allowances:    allowances,
		Deductions:    deductions,
		NetSalary:     netSalary,
		EffectiveDate: row.EffectiveDate,
		CreatedBy:     row.CreatedBy,
		CreatedAt:     row.CreatedAt,
	}, nil
}

func (s *CompensationService) mapBonusRow(row hrisrepo.BonusRow) (model.BonusRecord, error) {
	amount, err := decryptAmount(s.encrypter, row.Amount)
	if err != nil {
		return model.BonusRecord{}, err
	}
	return model.BonusRecord{
		ID:             row.ID,
		EmployeeID:     row.EmployeeID,
		Amount:         amount,
		Reason:         row.Reason,
		PeriodMonth:    row.PeriodMonth,
		PeriodYear:     row.PeriodYear,
		ApprovalStatus: row.ApprovalStatus,
		ApprovedBy:     row.ApprovedBy,
		ApprovedAt:     row.ApprovedAt,
		CreatedBy:      row.CreatedBy,
		CreatedAt:      row.CreatedAt,
	}, nil
}

func sumAmountMap(values map[string]int64) int64 {
	var total int64
	for _, amount := range values {
		total += amount
	}
	return total
}

func encryptAmountMap(encrypter *security.Encrypter, values map[string]int64) (string, error) {
	if values == nil {
		values = map[string]int64{}
	}
	payload, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return encrypter.EncryptString(string(payload))
}

func decryptAmountMap(encrypter *security.Encrypter, ciphertext string) (map[string]int64, error) {
	plaintext, err := encrypter.DecryptString(ciphertext)
	if err != nil {
		return nil, err
	}
	var result map[string]int64
	if err := json.Unmarshal([]byte(plaintext), &result); err != nil {
		return nil, err
	}
	if result == nil {
		result = map[string]int64{}
	}
	return result, nil
}

func decryptAmount(encrypter *security.Encrypter, ciphertext string) (int64, error) {
	plaintext, err := encrypter.DecryptString(ciphertext)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(plaintext, 10, 64)
}
