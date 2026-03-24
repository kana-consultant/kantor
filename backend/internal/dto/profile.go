package dto

type ChangeEmailRequest struct {
	Email    string `json:"email" validate:"required,email,max=255"`
	Password string `json:"password" validate:"required"`
}

type UpdateProfileRequest struct {
	FullName          string  `json:"full_name" validate:"required,min=3,max=150"`
	Phone             *string `json:"phone" validate:"omitempty,max=50"`
	Address           *string `json:"address" validate:"omitempty,max=500"`
	EmergencyContact  *string `json:"emergency_contact" validate:"omitempty,max=255"`
	AvatarURL         *string `json:"avatar_url" validate:"omitempty,max=500"`
	BankAccountNumber *string `json:"bank_account_number" validate:"omitempty,max=100"`
	BankName          *string `json:"bank_name" validate:"omitempty,max=100"`
	LinkedInProfile   *string `json:"linkedin_profile" validate:"omitempty,max=500"`
	SSHKeys           *string `json:"ssh_keys" validate:"omitempty,max=8000"`
}
