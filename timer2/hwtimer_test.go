package timer2

import (
	"context"
	"testing"
	"time"
)


func TestTimerWheel(t *testing.T) {
	timer, err := NewHashedWheelTimer(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	go timer.Start()
	defer timer.Stop()

	t.Log("start time: ", time.Now())
	tt := timer.Submit(3 * time.Second, func() {
		t.Log("timer task ", time.Now())
	})
	t.Log(tt.TID())

	time.Sleep(4 * time.Second + 100 * time.Millisecond)
}