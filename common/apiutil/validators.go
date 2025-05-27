package apiutil

import (
	"reflect"
	"strings"

	"github.com/go-playground/validator"
	"github.com/litebittech/cex/common/errors"
)

func NewValidator() *Validator {
	validator := validator.New()
	validator.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})

	return &Validator{validator}
}

type Validator struct {
	validator *validator.Validate
}

func (v *Validator) Validate(i interface{}) error {
	if err := v.validator.Struct(i); err != nil {
		validationErr := errors.Invalid.Explain("validation error")
		var fieldsError validator.ValidationErrors
		if errors.As(err, &fieldsError) {
			for _, fieldErr := range fieldsError {
				validationErr = validationErr.WithField(fieldErr.Tag(), fieldErr.Field(), "")
			}
		}
		return validationErr
	}
	return nil
}
