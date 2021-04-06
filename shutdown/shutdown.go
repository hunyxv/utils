package shutdown

import (
	"os"
	"os/signal"
	"syscall"
)

var _ Hook = (*hook)(nil)

// Hook a graceful shutdown hook, default with signals of SIGINT and SIGTERM
type Hook interface {
	// WithSignals add more signals into hook
	WithSignals(signals ...syscall.Signal)

	// ADD register shutdown handles
	Add(func())

	// WatchSignal with signal
	WatchSignal()
}

type hook struct {
	ctx    chan os.Signal
	hadles []func()
}

// NewHook create a Hook instance
func NewHook() Hook {
	hook := &hook{
		ctx: make(chan os.Signal, 1),
	}
	hook.Add(func() {
		signal.Stop(hook.ctx)
	})
	hook.WithSignals(syscall.SIGINT, syscall.SIGTERM)
	return hook
}

func (h *hook) WithSignals(signals ...syscall.Signal) {
	for _, s := range signals {
		signal.Notify(h.ctx, s)
	}
}

func (h *hook) Add(f func()) {
	h.hadles = append(h.hadles, f)
}

func (h *hook) WatchSignal() {
	<-h.ctx
	for _, h := range h.hadles {
		h()
	}
}
