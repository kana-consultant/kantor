package hris

import "time"

type CreateEmployeeRequest struct {
	FullName          string    `json:"full_name" validate:"required,min=3,max=150"`
	Email             string    `json:"email" validate:"required,email,max=255"`
	Phone             *string   `json:"phone" validate:"omitempty,max=50"`
	Position          string    `json:"position" validate:"required,min=2,max=100"`
	Department        *string   `json:"department" validate:"omitempty,max=100"`
	DateJoined        time.Time `json:"date_joined" validate:"required"`
	EmploymentStatus  string    `json:"employment_status" validate:"required,oneof=active probation resigned terminated"`
	Address           *string   `json:"address" validate:"omitempty,max=500"`
	EmergencyContact  *string   `json:"emergency_contact" validate:"omitempty,max=255"`
	AvatarURL         *string   `json:"avatar_url" validate:"omitempty,max=500"`
	BankAccountNumber *string   `json:"bank_account_number" validate:"omitempty,max=100"`
	BankName          *string   `json:"bank_name" validate:"omitempty,max=100"`
	LinkedInProfile   *string   `json:"linkedin_profile" validate:"omitempty,max=500"`
	SSHKeys           *string   `json:"ssh_keys" validate:"omitempty,max=8000"`
}

type UpdateEmployeeRequest struct {
	FullName          string    `json:"full_name" validate:"required,min=3,max=150"`
	Email             string    `json:"email" validate:"required,email,max=255"`
	Phone             *string   `json:"phone" validate:"omitempty,max=50"`
	Position          string    `json:"position" validate:"required,min=2,max=100"`
	Department        *string   `json:"department" validate:"omitempty,max=100"`
	DateJoined        time.Time `json:"date_joined" validate:"required"`
	EmploymentStatus  string    `json:"employment_status" validate:"required,oneof=active probation resigned terminated"`
	Address           *string   `json:"address" validate:"omitempty,max=500"`
	EmergencyContact  *string   `json:"emergency_contact" validate:"omitempty,max=255"`
	AvatarURL         *string   `json:"avatar_url" validate:"omitempty,max=500"`
	BankAccountNumber *string   `json:"bank_account_number" validate:"omitempty,max=100"`
	BankName          *string   `json:"bank_name" validate:"omitempty,max=100"`
	LinkedInProfile   *string   `json:"linkedin_profile" validate:"omitempty,max=500"`
	SSHKeys           *string   `json:"ssh_keys" validate:"omitempty,max=8000"`
}

type ListEmployeesQuery struct {
	Page             int    `validate:"omitempty,min=1"`
	PerPage          int    `validate:"omitempty,min=1,max=100"`
	Search           string `validate:"omitempty,max=150"`
	Department       string `validate:"omitempty,max=100"`
	EmploymentStatus string `validate:"omitempty,oneof=active probation resigned terminated"`
}
