package heaptimer

import (
	"log"

	"github.com/panjf2000/ants/v2"
)

type Option func(*options)

type options struct {
	Logger   Logger // logger
	WorkPool *ants.Pool
}

// WithLogger 日志
// 	默认为 log.Logger
func WithLogger(l Logger) Option {
	return func(o *options) {
		o.Logger = l
	}
}

// WithWorkPool 工作池
// 	默认无大小限制
func WithWorkPool(p *ants.Pool) Option {
	return func(o *options) {
		o.WorkPool = p
	}
}

type Logger interface {
	Warn(args ...interface{})
	Error(args ...interface{})
}

type defaultLogger struct {
	*log.Logger
}

func (l *defaultLogger) Warn(args ...interface{}) {
	l.Println(args...)
}

func (l *defaultLogger) Error(args ...interface{}) {
	l.Println(args...)
}
