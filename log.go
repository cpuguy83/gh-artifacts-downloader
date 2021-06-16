package main

import (
	"context"

	"github.com/sirupsen/logrus"
)

type logCtx struct{}

func withLogger(ctx context.Context, l *logrus.Entry) context.Context {
	return context.WithValue(ctx, logCtx{}, l)
}

var baseLogger = logrus.NewEntry(logrus.StandardLogger())

func logger(ctx context.Context) *logrus.Entry {
	l := ctx.Value(logCtx{})
	if l != nil {
		return l.(*logrus.Entry)
	}
	return baseLogger
}
