package hris

import (
	"net/http"

	"github.com/go-playground/validator/v10"

	"github.com/kana-consultant/kantor/backend/internal/httputil"
)

func newValidator() *validator.Validate {
	return httputil.NewValidator()
}

func decodeAndValidate(v *validator.Validate, w http.ResponseWriter, r *http.Request, target interface{}) bool {
	return httputil.DecodeAndValidate(v, w, r, target)
}

func validationDetails(err error) map[string]string {
	return httputil.ValidationDetails(err)
}
