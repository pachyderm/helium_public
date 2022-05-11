// Package sentry implements utility functions related to Sentry use in Hub.
package sentry

import (
	"fmt"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/pachyderm/helium/terrors"
	"github.com/sirupsen/logrus"
)

// Logrus is a logrus hook that sends errors to Sentry.  You are responsible for initializing
// Sentry's global state.
type Logrus struct {
	EnabledLevels []logrus.Level
}

// Levels implements a Logrus hook.
func (l *Logrus) Levels() []logrus.Level {
	return l.EnabledLevels
}

var sentryLevel = map[logrus.Level]sentry.Level{
	logrus.TraceLevel: sentry.LevelDebug,
	logrus.DebugLevel: sentry.LevelDebug,
	logrus.InfoLevel:  sentry.LevelInfo,
	logrus.WarnLevel:  sentry.LevelWarning,
	logrus.ErrorLevel: sentry.LevelError,
	logrus.PanicLevel: sentry.LevelFatal,
	logrus.FatalLevel: sentry.LevelFatal,
}

// Unfortunately, this doesn't work perfectly.  It's 6 frames between entry.Error(...) and
// hook.Fire(), but 7 frames from logrus.Error(...) -- there is a frame in there to create an entry
// from the default logger when you log to it.  Sentry filters that out, but it is a little
// annoying.
const logrusDepth = 6

// Fire implements a Logrus hook.
func (l *Logrus) Fire(entry *logrus.Entry) error {
	var err error
	if rawErr, ok := entry.Data[logrus.ErrorKey].(error); ok {
		// Generate an error from a statement like logrus.WithError(err).Error("message").
		err = terrors.WrapN(fmt.Errorf("%s: %w", entry.Message, rawErr), logrusDepth)
	} else {
		// There was no log.WithError(err) error in the entry, so generate a new error from
		// the logged message.
		err = terrors.NewN(entry.Message, logrusDepth)
	}

	sentry.WithScope(func(s *sentry.Scope) {
		s.SetLevel(sentryLevel[entry.Level])
		for k, v := range entry.Data {
			if k == logrus.ErrorKey {
				continue
			}
			switch x := v.(type) {
			case string:
				s.SetTag(k, x)
			case []byte:
				s.SetTag(k, string(x))
			case int, int8, int32, int64, uint, uint8, uint32, uint64, float32, float64, bool:
				s.SetTag(k, fmt.Sprintf("%v", v))
			case time.Time:
				s.SetTag(k, x.In(time.UTC).Format(time.RFC3339))
			default:
				s.SetTag(k, fmt.Sprintf("%#v", v))
			}
		}
		sentry.CaptureException(err)
	})
	return nil
}
