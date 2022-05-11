// Package terrors provides an error type that captures stack traces.
package terrors

import (
	"errors"
	"fmt"
	"io"
	"runtime"
)

// TracedError is an error that captures a stack trace at the original point of failure.  It
// implements the exact API that Sentry needs to extract stack traces, even if it is wrapped in
// another error (or it wraps another error).  (Sentry extracts stack traces from errors with
// reflection; they offer no API for this but instead attempted to reverse-engineer a few random
// open source projects that happen to also collect stack traces.  All of those libraries have weird
// quirks, so we just pretend to be one of them when Sentry tries to reflect, rather than actually
// using them.  That way we get the Sentry integration, but none of the quirks.)
//
// Do not create a TracedError directly; use a constructor.
type TracedError struct {
	err error     // underlying error
	pcs []uintptr // stack trace in the form of program counters
}

func trace(skip int) []uintptr {
	pc := make([]uintptr, 32)
	got := runtime.Callers(2+skip, pc) // runtime.Callers + this function = 2
	if got < len(pc) {
		pc = pc[:got]
	}
	return pc
}

// New returns an error containing a stack trace from the perspective of the caller.
//
// Note that errors.Is(New("test"), errors.New("test")) returns false.  To use errors.Is with an
// object returned from errors.New, you must Wrap the exact object you intend to compare with.
// (errors.Is(Wrap(os.ErrNoExist), os.ErrNoExist) is true.)  This is not an implementation detail of
// this library; errors.Is(errors.New("test"), errors.New("test")) always returns false.
func New(msg string) error {
	return NewN(msg, 1) // skip New.
}

// NewN returns an error containing a stack trace from the perspective of "skip" frames above the
// caller.
func NewN(msg string, skip int) error {
	return &TracedError{
		err: errors.New(msg),
		pcs: trace(1 + skip), // skip NewN.
	}
}

// Wrap wraps an existing error with an error containing the stack trace at the point where Wrap was
// called.
func Wrap(err error) error {
	return WrapN(err, 1) // skip Wrap.
}

// WrapN wraps an existing error with an error containing the stack trace, skipping "skip" frames.
// 0 frames to skip means to start from the perspective of the caller.
func WrapN(err error, skip int) error {
	return &TracedError{
		err: err,
		pcs: trace(1 + skip), // skip WrapN.
	}
}

// Errorf builds an error message from a format string (the same as fmt.Errorf), and includes a
// stack trace from the perspective of the caller.
func Errorf(format string, args ...interface{}) error {
	return WrapN(fmt.Errorf(format, args...), 1) // skip Errorf.
}

// Error implements error.
func (err *TracedError) Error() string {
	return err.err.Error()
}

// StackTrace implements a random reflection call in the Sentry library (see
// sentry.extractReflectedStacktraceMethod).
func (err *TracedError) StackTrace() []uintptr {
	return err.pcs
}

// PrintStack prints the stack trace contained in a TracedError, for debugging.
func (err *TracedError) PrintStack(w io.Writer) {
	frames := runtime.CallersFrames(err.StackTrace())
	for {
		frame, ok := frames.Next()
		if !ok {
			break
		}
		fmt.Fprintf(w, "    [%v] %v\n         %v:%v\n", frame.PC, frame.Function, frame.File, frame.Line)
	}
}

// Unwrap implements errors.Unwrap.  (Unwrap then implements Is and As.)
func (err *TracedError) Unwrap() error {
	return err.err
}
