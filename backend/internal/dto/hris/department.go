package hris

type CreateDepartmentRequest struct {
	Name        string  `json:"name" validate:"required,min=2,max=100"`
	Description *string `json:"description" validate:"omitempty,max=500"`
	HeadID      *string `json:"head_id" validate:"omitempty,uuid4"`
}

type UpdateDepartmentRequest struct {
	Name        string  `json:"name" validate:"required,min=2,max=100"`
	Description *string `json:"description" validate:"omitempty,max=500"`
	HeadID      *string `json:"head_id" validate:"omitempty,uuid4"`
}
