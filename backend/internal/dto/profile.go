package dto

type UpdateProfileRequest struct {
	FullName         string  `json:"full_name" validate:"required,min=3,max=150"`
	Phone            *string `json:"phone" validate:"omitempty,max=50"`
	Address          *string `json:"address" validate:"omitempty,max=500"`
	EmergencyContact *string `json:"emergency_contact" validate:"omitempty,max=255"`
	AvatarURL        *string `json:"avatar_url" validate:"omitempty,url,max=500"`
}
