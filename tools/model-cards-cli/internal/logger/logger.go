package logger

import (
	"github.com/sirupsen/logrus"
)

var (
	// Log is the default logger instance
	Log *logrus.Logger
)

// Fields type is an alias for logrus.Fields
type Fields logrus.Fields

func init() {
	Log = logrus.New()
	Log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
	})
}

// Debugf logs a formatted message at level Debug
func Debugf(format string, args ...interface{}) {
	Log.Debugf(format, args...)
}

// Info logs a message at level Info
func Info(args ...interface{}) {
	Log.Info(args...)
}

// Infof logs a formatted message at level Info
func Infof(format string, args ...interface{}) {
	Log.Infof(format, args...)
}

// Warnf logs a formatted message at level Warn
func Warnf(format string, args ...interface{}) {
	Log.Warnf(format, args...)
}

// WithField creates an entry from the standard logger and adds a field to it
func WithField(key string, value interface{}) *logrus.Entry {
	return Log.WithField(key, value)
}

// WithFields creates an entry from the standard logger and adds multiple fields to it
func WithFields(fields Fields) *logrus.Entry {
	return Log.WithFields(logrus.Fields(fields))
}

// WithError creates an entry from the standard logger and adds an error to it
func WithError(err error) *logrus.Entry {
	return Log.WithError(err)
}
