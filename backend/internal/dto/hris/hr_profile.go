package hris

import "time"

// UpdateHRProfileRequest edits an employee's HR profile. Only the groups that
// are present are changed:
//   - job_title needs hris:employee:edit ("" clears it);
//   - personal_email and identity need hris:employee_identity:edit.
//
// identity replaces every identity field except the NIK: an empty nik keeps
// the stored one (it is never sent back to the client, so it can only be
// replaced, not edited). A caller without hris:employee_identity:view cannot
// keep it: its identity save is a full replacement and nik is required.
type UpdateHRProfileRequest struct {
	JobTitle      *string                  `json:"job_title" validate:"omitempty,max=150"`
	PersonalEmail *string                  `json:"personal_email" validate:"omitempty,max=160"`
	Identity      *UpdateHRIdentityRequest `json:"identity"`
}

type UpdateHRIdentityRequest struct {
	NIK             string `json:"nik" validate:"max=32"`
	BirthPlace      string `json:"birth_place" validate:"max=100"`
	BirthDate       string `json:"birth_date" validate:"omitempty,datetime=2006-01-02"`
	Gender          string `json:"gender" validate:"omitempty,oneof=male female"`
	BankAccountName string `json:"bank_account_name" validate:"max=150"`
	KTPAddress      string `json:"ktp_address" validate:"max=500"`
}

// HRProfileResponse is visible to hris:employee:view. Identity and
// PersonalEmail are present only when IdentityVisible (the caller holds
// hris:employee_identity:view); every such response is access-logged.
type HRProfileResponse struct {
	EmployeeID      string              `json:"employee_id"`
	EmployeeNumber  *int                `json:"employee_number"`
	EmployeeCode    *string             `json:"employee_code"`
	JobTitle        *string             `json:"job_title"`
	UpdatedAt       *time.Time          `json:"updated_at"`
	IdentityVisible bool                `json:"identity_visible"`
	PersonalEmail   *string             `json:"personal_email,omitempty"`
	Identity        *HRIdentityResponse `json:"identity,omitempty"`
}

// HRIdentityResponse never carries the full NIK: NIKMasked keeps the six
// region digits and hides the rest ("327301**********"). Empty strings mean
// "not filled in".
type HRIdentityResponse struct {
	HasNIK          bool       `json:"has_nik"`
	NIKMasked       string     `json:"nik_masked"`
	BirthPlace      string     `json:"birth_place"`
	BirthDate       string     `json:"birth_date"`
	Gender          string     `json:"gender"`
	BankAccountName string     `json:"bank_account_name"`
	KTPAddress      string     `json:"ktp_address"`
	UpdatedAt       *time.Time `json:"updated_at"`
}
