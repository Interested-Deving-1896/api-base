// Package validator wraps go-playground/validator with a helper that
// translates validation errors into our standard response.FieldError
// format.
//
// Handlers call c.ShouldBindJSON(&req), which internally runs the
// validator against the DTO's `binding:` struct tags. If validation
// fails, the handler passes the error to TranslateErrors and then to
// response.ValidationError so the client gets a structured, field-level
// error response.
package validator

import (
	"errors"
	"fmt"

	govalidator "github.com/go-playground/validator/v10"
	"github.com/topboyasante/api-base/internal/shared/response"
)

func New() *govalidator.Validate {
	return govalidator.New()
}

// TranslateErrors converts a go-playground/validator error into our
// FieldError slice format. If the error isn't a ValidationErrors, returns nil.
func TranslateErrors(err error) []response.FieldError {
	var ve govalidator.ValidationErrors
	if !errors.As(err, &ve) {
		return nil
	}
	out := make([]response.FieldError, 0, len(ve))
	for _, fe := range ve {
		out = append(out, response.FieldError{
			Field:   fe.Field(),
			Tag:     fe.Tag(),
			Message: messageFor(fe),
		})
	}
	return out
}

func messageFor(fe govalidator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return fmt.Sprintf("%s is required", fe.Field())
	case "email":
		return fmt.Sprintf("%s must be a valid email", fe.Field())
	case "min":
		return fmt.Sprintf("%s must be at least %s characters", fe.Field(), fe.Param())
	case "max":
		return fmt.Sprintf("%s must be at most %s characters", fe.Field(), fe.Param())
	default:
		return fmt.Sprintf("%s failed %s validation", fe.Field(), fe.Tag())
	}
}
