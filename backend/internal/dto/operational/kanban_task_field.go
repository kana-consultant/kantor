package operational

type KanbanTaskFieldInput struct {
	Name  string `json:"name" validate:"required,min=1,max=120"`
	Value string `json:"value" validate:"max=20000"`
}
