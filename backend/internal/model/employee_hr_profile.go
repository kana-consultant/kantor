package model

import "time"

// EmployeeHRProfile is the employee_hr_profiles row. IdentityEncrypted is the
// security.Encrypter ciphertext of an EmployeeIdentity JSON document and is
// never serialised to clients.
type EmployeeHRProfile struct {
	EmployeeID        string
	EmployeeNumber    *int
	EmployeeCode      *string
	JobTitle          *string
	PersonalEmail     *string
	IdentityEncrypted *string
	IdentityUpdatedAt *time.Time
	UpdatedBy         *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// Gender values stored in EmployeeIdentity.Gender.
const (
	GenderMale   = "male"
	GenderFemale = "female"
)

// EmployeeIdentity is the decrypted identity blob. BirthDate is YYYY-MM-DD.
type EmployeeIdentity struct {
	NIK             string `json:"nik"`
	BirthPlace      string `json:"birth_place"`
	BirthDate       string `json:"birth_date"`
	Gender          string `json:"gender"`
	BankAccountName string `json:"bank_account_name"`
	KTPAddress      string `json:"ktp_address"`
}
