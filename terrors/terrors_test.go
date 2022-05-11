package terrors

import (
	"bytes"
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestAPI(t *testing.T) {
	prototype := errors.New("test")
	testData := []struct {
		name   string
		f      func() error
		wantIs []error
	}{
		{
			name: "New",
			f: func() error {
				return New("test")
			},
		},
		{
			name: "NewN",
			f: func() error {
				return NewN("test", 1) // skips f()
			},
		},
		{
			name: "Wrap",
			f: func() error {
				return Wrap(prototype)
			},
			wantIs: []error{prototype},
		},
		{
			name: "WrapN",
			f: func() error {
				return WrapN(prototype, 1) // skips f()
			},
			wantIs: []error{prototype},
		},
		{
			name: "Errorf",
			f: func() error {
				return Errorf("bad news: %w", prototype)
			},
			wantIs: []error{prototype},
		},
		{
			name: "Errorf and Wrap",
			f: func() error {
				return Errorf("much indirection: %w", Wrap(prototype))
			},
			wantIs: []error{prototype},
		},
		{
			name: "Wrap a library error",
			f: func() error {
				return Wrap(os.ErrNotExist)
			},
			wantIs: []error{os.ErrNotExist},
		},
		{
			name: "Errorf a library error",
			f: func() error {
				return Errorf("delete some made-up file: %w", os.ErrNotExist)
			},
			wantIs: []error{os.ErrNotExist},
		},
	}

	for _, test := range testData {
		t.Run(test.name, func(t *testing.T) {
			// Figure out the name of our own function, to ensure that it's in the stack
			// traces returned by the tests.  Because of how t.Run works, we can't just
			// look for "TestAPI".
			pcs := make([]uintptr, 10)
			n := runtime.Callers(1, pcs)
			frames := runtime.CallersFrames(pcs)
			frame, ok := frames.Next()
			if !ok {
				t.Fatalf("could not get our own first frame (pcs: %#v, n: %v)", pcs, n)
			}
			wantFunc := frame.Function

			// Get the error.
			err := test.f()

			// Check that a wrapped error Is everything the test requires.
			for _, want := range test.wantIs {
				if got := err; !errors.Is(got, want) {
					t.Errorf("error 'Is' not the prototype:\n  got: %#v\n want: %#v", got, want)
				}
				if got := err; !errors.As(got, &want) {
					t.Errorf("error cannot be cast:\n  got: %#v\n want: %#v", got, want)
				}
			}

			// Check that we can cast the wrapped error to a TracedError.
			tracedErr := &TracedError{}
			if !errors.As(err, &tracedErr) {
				t.Fatalf("cannot convert %#v to TracedError via errors.As", err)
			}

			// Format the stack trace to debug a failing test.
			buf := new(bytes.Buffer)
			tracedErr.PrintStack(buf)
			trace := buf.String()

			// Check that the error's stack trace includes this frame.
			var stackOK bool
			frames = runtime.CallersFrames(tracedErr.pcs)
			for {
				frame, ok := frames.Next()
				if !ok {
					break
				}
				gotFunc := frame.Function
				if got, want := gotFunc, wantFunc; got == want {
					stackOK = true
					break
				}
			}
			if !stackOK {
				t.Errorf("did not see function %q in error trace:\n  trace:\n%s", wantFunc, trace)
			}

			// Check that the text representation also contains this frame.
			if !strings.Contains(trace, wantFunc) {
				t.Errorf("text formatted stack trace does not contain %q:\n  trace:\n%s", wantFunc, trace)
			}
		})
	}
}
