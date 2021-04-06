package shutdown

import (
	"log"
	"testing"
)

type Server struct{}

func (s *Server) Close() {
	log.Fatal("shutting down")
}

func TestShutdown(t *testing.T) {
	hook := NewHook()
	server1 := &Server{}
	hook.Add(func() { server1.Close() })
	server2 := &Server{}
	hook.Add(func() { server2.Close() })

	hook.WatchSignal()
}
