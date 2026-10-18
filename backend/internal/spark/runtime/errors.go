package runtime

import (
	"errors"

	"campfire/internal/shared/model"
)

type runtimeFailure struct {
	code    model.RuntimeErrorCode
	cause   error
	message string
}

func (e *runtimeFailure) Error() string                            { return e.message }
func (e *runtimeFailure) Unwrap() error                            { return e.cause }
func (e *runtimeFailure) RuntimeErrorCode() model.RuntimeErrorCode { return e.code }

func runtimeErrorCode(err error) model.RuntimeErrorCode {
	var failure interface{ RuntimeErrorCode() model.RuntimeErrorCode }
	if errors.As(err, &failure) {
		return failure.RuntimeErrorCode()
	}

	return model.RuntimeOperationFailed
}
