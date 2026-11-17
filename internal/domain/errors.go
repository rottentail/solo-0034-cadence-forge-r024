package domain

import (
	"errors"
	"fmt"
)

// Fault is a client-correctable rejection: the input cannot be processed as
// sent. Status, code and message are the external contract and stay stable.
type Fault struct {
	Status  int
	Code    string
	Message string
}

func (e *Fault) Error() string { return e.Message }
func Invalid(format string, args ...any) error {
	return &Fault{400, "invalid", fmt.Sprintf(format, args...)}
}
func Missing(kind, id string) error {
	return &Fault{404, "not_found", fmt.Sprintf("%s %s does not exist", kind, id)}
}
func Conflict(message string) error { return &Fault{409, "conflict", message} }
func Capacity(message string) error { return &Fault{429, "capacity", message} }

// Kind classifies failures the client cannot correct by changing the input.
type Kind string

const (
	// KindEnvironment marks runtime environment failures: media processes,
	// filesystem and resource bounds. The request may succeed once the
	// environment recovers.
	KindEnvironment Kind = "environment"
	// KindPersistence marks uncertain durability: the change may already be
	// committed, so it must never be reported as simply failed.
	KindPersistence Kind = "persistence"
)

// Failure is an internal error that keeps its diagnostics: Kind separates
// environment failures from uncertain persistence, Stage names the
// processing step (receive, probe, encode, promote, commit, ...) so ingest
// and render failures stay explainably different, and Cause keeps the chain.
type Failure struct {
	Kind    Kind
	Stage   string
	Message string
	Cause   error
}

func (f *Failure) Error() string {
	if f.Cause != nil {
		return f.Stage + ": " + f.Message + ": " + f.Cause.Error()
	}
	return f.Stage + ": " + f.Message
}
func (f *Failure) Unwrap() error { return f.Cause }

// Env reports a runtime environment failure at the given stage.
func Env(stage, message string, cause error) error {
	return &Failure{Kind: KindEnvironment, Stage: stage, Message: message, Cause: cause}
}

// Uncertain reports that a change may be committed while its durability is
// unproven; the outcome must be reconciled, never assumed to have failed.
func Uncertain(stage, message string, cause error) error {
	return &Failure{Kind: KindPersistence, Stage: stage, Message: message, Cause: cause}
}

// FailureOf extracts the outermost Failure for structured diagnostics.
func FailureOf(err error) *Failure {
	var f *Failure
	if errors.As(err, &f) {
		return f
	}
	return nil
}

// Classify maps an error onto the external status/code/message contract.
// The Failure check comes first: a Failure wrapping a Fault (for example a
// state invariant rejecting stored input) is a server-side problem and must
// not surface as a client-correctable status.
func Classify(err error) (int, string, string) {
	var f *Failure
	if errors.As(err, &f) {
		if f.Kind == KindPersistence {
			return 500, "persistence_uncertain", "operation outcome uncertain; consult the server event log before retrying"
		}
		return 500, "internal", "operation failed; consult the server event log"
	}
	var fault *Fault
	if errors.As(err, &fault) {
		return fault.Status, fault.Code, fault.Message
	}
	return 500, "internal", "operation failed; consult the server event log"
}
