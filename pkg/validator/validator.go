package validator

import (
	"errors"
	"strings"

	"github.com/go-playground/validator/v10"

	"wanpey/core/pkg/apperror"
)

type EchoValidator struct {
	v *validator.Validate
}

func New() *EchoValidator {
	return &EchoValidator{v: validator.New()}
}

func (ev *EchoValidator) Validate(i any) error {
	if err := ev.v.Struct(i); err != nil {
		var errs validator.ValidationErrors
		if !errors.As(err, &errs) {
			return apperror.BadRequest("invalid request body")
		}
		details := make([]apperror.FieldError, 0, len(errs))
		for _, fe := range errs {
			details = append(details, apperror.FieldError{
				Field:   strings.ToLower(fe.Field()),
				Message: fieldMessage(fe),
			})
		}
		return apperror.BadRequest("validation failed").WithDetails(details...)
	}
	return nil
}

func fieldMessage(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "required"
	case "email":
		return "must be a valid email"
	case "url":
		return "must be a valid URL"
	case "min":
		return "minimum " + fe.Param()
	case "max":
		return "maximum " + fe.Param()
	case "oneof":
		return "must be one of: " + fe.Param()
	case "gt":
		return "must be greater than " + fe.Param()
	default:
		return "invalid value"
	}
}
