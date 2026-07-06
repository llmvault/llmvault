package apps

import "errors"

// InfraError marks a deploy failure as our-side infrastructure (sandbox create,
// the gateway/alias push, storage, transport) rather than a fault in the agent's
// app bundle, so the MCP layer can tell the agent to stop and apologise instead
// of retrying a failure that will not clear on its own.
type InfraError struct{ Err error }

func (e *InfraError) Error() string { return e.Err.Error() }
func (e *InfraError) Unwrap() error { return e.Err }

// classifyDeployFault tags a deploy failure by fault: an AppdError (appd
// rejected the bundle or the app would not start) and a ValidationError are the
// caller's and pass through; everything else is our infrastructure, wrapped as
// InfraError.
func classifyDeployFault(err error) error {
	if err == nil {
		return nil
	}
	var appdErr *AppdError
	if errors.As(err, &appdErr) {
		return err
	}
	var validationErr *ValidationError
	if errors.As(err, &validationErr) {
		return err
	}
	return &InfraError{Err: err}
}
