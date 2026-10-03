package model

import "time"

type Employee struct {
	ID                string    `json:"id"`
	UserID            *string   `json:"user_id,omitempty"`
	FullName          string    `json:"full_name"`
	Email             string    `json:"email"`
	Phone             *string   `json:"phone,omitempty"`
	Position          string    `json:"position"`
	Department        *string   `json:"department,omitempty"`
	DateJoined        time.Time `json:"date_joined"`
	EmploymentStatus  string    `json:"employment_status"`
	Address           *string   `json:"address,omitempty"`
	EmergencyContact  *string   `json:"emergency_contact,omitempty"`
	AvatarURL         *string   `json:"avatar_url,omitempty"`
	BankAccountNumber *string   `json:"bank_account_number,omitempty"`
	// BankAccountUnreadable: a stored (encrypted) account number exists but
	// could not be decrypted; BankAccountNumber is then nil. Never sent.
	BankAccountUnreadable bool      `json:"-"`
	BankName              *string   `json:"bank_name,omitempty"`
	LinkedInProfile       *string   `json:"linkedin_profile,omitempty"`
	SSHKeys               *string   `json:"ssh_keys,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type Department struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	HeadID      *string   `json:"head_id,omitempty"`
	HeadName    *string   `json:"head_name,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}
