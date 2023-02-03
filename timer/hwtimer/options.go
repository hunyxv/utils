package hwtimer

import (
	"time"

	"github.com/panjf2000/ants/v2"
)

type Option func(opts *Options)

type Options struct {
	// TickDuration 一个 bucket 的时间跨度，默认为 100ms
	TickDuration time.Duration
	// TicksPerWheel 一轮含有多少个 bucket ，默认为 512 个
	TicksPerWheel int32
	// WorkPool 工作池
	WorkPool *ants.Pool
}

// WithTickDuration 设置 bucket 时间跨度，默认 100ms
func WithTickDuration(tickDuration time.Duration) Option {
	return func(opts *Options) {
		opts.TickDuration = tickDuration
	}
}

// WithTicksPerWheel 设置时间轮刻度数量
func WithTicksPerWheel(ticksPerWheel int32) Option {
	return func(opts *Options) {
		opts.TicksPerWheel = ticksPerWheel
	}
}

// WithWorkPool 设置工作池，默认工作池无限大
func WithWorkPool(pool *ants.Pool) Option {
	return func(opts *Options) {
		opts.WorkPool = pool
	}
}
