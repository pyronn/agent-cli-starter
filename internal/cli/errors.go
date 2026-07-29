package cli

import "fmt"

type exitError struct {
	exitCode int
	code     string
	message  string
	cause    error
}

func (e *exitError) Error() string {
	return e.message
}

func (e *exitError) Unwrap() error {
	return e.cause
}

func newExitError(exitCode int, code, message string, cause error) error {
	return &exitError{
		exitCode: exitCode,
		code:     code,
		message:  message,
		cause:    cause,
	}
}

func usageError(err error) error {
	return newExitError(2, "usage_error", err.Error(), err)
}

func configError(err error) error {
	return newExitError(2, "config_error", fmt.Sprintf("configuration error: %v", err), err)
}

func runtimeError(err error) error {
	return newExitError(1, "runtime_error", err.Error(), err)
}
