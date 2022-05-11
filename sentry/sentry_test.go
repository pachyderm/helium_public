package sentry

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/pachyderm/helium/terrors"
	"github.com/sirupsen/logrus"
)

type fakeTransport struct {
	ch chan *sentry.Event
}

func (t *fakeTransport) Flush(timeout time.Duration) bool       { return true }
func (t *fakeTransport) Configure(options sentry.ClientOptions) {}
func (t *fakeTransport) SendEvent(event *sentry.Event)          { t.ch <- event }

type someStruct struct {
	Foo string
}

func TestLogrus(t *testing.T) {
	ch := make(chan *sentry.Event, 1)
	if err := sentry.Init(sentry.ClientOptions{Transport: &fakeTransport{ch: ch}}); err != nil {
		t.Fatalf("init sentry: %v", err)
	}
	buf := new(bytes.Buffer)
	l := &logrus.Logger{
		Out: buf,
		Formatter: &logrus.TextFormatter{
			DisableColors:    true,
			DisableTimestamp: true,
		},
		Level: logrus.DebugLevel,
		Hooks: make(logrus.LevelHooks),
	}
	l.AddHook(&Logrus{EnabledLevels: []logrus.Level{logrus.ErrorLevel}})
	testData := []struct {
		name       string
		log        func()
		wantLog    string
		wantErrors []string
		wantTags   map[string]string
	}{
		{
			name:       "bare error message",
			log:        func() { l.Error("bare message") },
			wantLog:    `level=error msg="bare message"`,
			wantErrors: []string{"bare message", "bare message (with trace)"},
		},
		{
			name:    "bare debug message",
			log:     func() { l.Debug("bare message") },
			wantLog: `level=debug msg="bare message"`,
		},
		{
			name:       "error message with error object",
			log:        func() { l.WithError(errors.New("error")).Error("message") },
			wantLog:    `level=error msg=message error=error`,
			wantErrors: []string{"error", "message: error", "message: error (with trace)"},
		},
		{
			name:    "debug message with error object",
			log:     func() { l.WithError(errors.New("error")).Debug("message") },
			wantLog: `level=debug msg=message error=error`,
		},
		{
			name:       "bare error message with fields",
			log:        func() { l.WithField("foo", "bar").Error("bare message") },
			wantLog:    `level=error msg="bare message" foo=bar`,
			wantErrors: []string{"bare message", "bare message (with trace)"},
			wantTags:   map[string]string{"foo": "bar"},
		},
		{
			name:       "error message with error object and fields",
			log:        func() { l.WithField("foo", "bar").WithError(errors.New("error")).Error("message") },
			wantLog:    `level=error msg=message error=error foo=bar`,
			wantErrors: []string{"error", "message: error", "message: error (with trace)"},
			wantTags:   map[string]string{"foo": "bar"},
		},
		{
			name: "lots of field types",
			log: func() {
				l.WithFields(logrus.Fields{
					"int":     42,
					"float64": 3.14,
					"bool":    true,
					"struct":  &someStruct{Foo: "hi"},
					"[]byte":  []byte("hello"),
					"string":  "world",
					"stamp":   time.Date(2020, 1, 2, 3, 45, 56, 54321, time.UTC),
				}).Error("message")
			},
			wantLog:    `level=error msg=message []byte="[104 101 108 108 111]" bool=true float64=3.14 int=42 stamp="2020-01-02 03:45:56.000054321 +0000 UTC" string=world struct="&{hi}"`,
			wantErrors: []string{"message", "message (with trace)"},
			wantTags: map[string]string{
				"int":     "42",
				"float64": "3.14",
				"bool":    "true",
				"struct":  `&sentry.someStruct{Foo:"hi"}`,
				"[]byte":  "hello",
				"string":  "world",
				"stamp":   "2020-01-02T03:45:56Z",
			},
		},
		{
			name:       "error message with traced error object",
			log:        func() { l.WithError(terrors.New("error")).Error("message") },
			wantLog:    `level=error msg=message error=error`,
			wantErrors: []string{"error", "error (with trace)", "message: error", "message: error (with trace)"},
		},
	}

	for _, test := range testData {
		t.Run(test.name, func(t *testing.T) {
			buf.Reset()
		clear:
			for {
				select {
				case <-ch:
				default:
					break clear
				}
			}
			test.log()

			var gotErrors []string
			var gotTags map[string]string
			var event *sentry.Event
			select {
			case event = <-ch:
			case <-time.After(100 * time.Millisecond):
			}
			if event != nil {
				for _, ex := range event.Exception {
					msg := ex.Value
					if ex.Stacktrace != nil {
						msg += " (with trace)"
					}
					gotErrors = append(gotErrors, msg)
				}
				gotTags = event.Tags
			}

			if diff := cmp.Diff(gotErrors, test.wantErrors, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("event:\n%s", diff)
			}

			if diff := cmp.Diff(gotTags, test.wantTags, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("tags:\n%s", diff)
			}

			gotLog := buf.String()
			if diff := cmp.Diff(strings.TrimSpace(gotLog), strings.TrimSpace(test.wantLog)); diff != "" {
				t.Errorf("message:\n%s", diff)
			}
		})
	}
}
