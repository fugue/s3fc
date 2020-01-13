package logging

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/sirupsen/logrus"
)

type Fields = logrus.Fields

// New creates a new logrus.FieldLogger configured for use in a Lambda Function.
func NewJsonLogger() logrus.FieldLogger {
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})
	return logger
}

// New creates a new logrus.FieldLogger configured for use in a Lambda Function.
func New() logrus.FieldLogger {
	logger := NewJsonLogger()
	return logger.WithFields(logrus.Fields{
		"service":          lambdacontext.FunctionName,
		"function_version": lambdacontext.FunctionVersion,
	})
}

// NewEventLogger creates a new logger configured for a specific invocation of a
// Lambda Function.
func NewEventLogger(ctx context.Context, logger logrus.FieldLogger) logrus.FieldLogger {
	if lc, ok := lambdacontext.FromContext(ctx); ok {
		return logger.WithField("request_id", lc.AwsRequestID)
	}
	logger.WithField("method", "NewLambdaEventLogger").Debug("missing lambda context")
	return logger
}

// SetOutput sets the output of the provided logger to the provided io.Writer.
// Mainly useful for disabling log output in unit tests.
//
// Note that only *logrus.Logger and *logrus.Entry loggers are supported.
func SetOutput(logger logrus.FieldLogger, w io.Writer) {
	switch v := logger.(type) {
	case *logrus.Logger:
		v.Out = w
	case *logrus.Entry:
		v.Logger.Out = w
	default:
		logger.WithField("logger_type", fmt.Sprintf("%T", logger)).Error("unknown logger type")
	}
}
