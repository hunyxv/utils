package timer

import (
	"math/rand"
	"testing"
	"time"
)

var _ TimerTask = (*task)(nil)

type task struct {
	t          *testing.T
	startTime  time.Time
	after      time.Duration
	needCancel bool
}

func (t *task) Run() error {
	t.t.Logf("starttime: %s \n\tafter:%d \n\trun: %s\n-------------", t.startTime.Format("2006/01/02 15:04:05"), t.after/time.Second, time.Now().Format("2006/01/02 15:04:05"))
	return nil
}

func (t *task) Exception(error) {

}

func (t *task) NeedCancel() bool {
	return t.needCancel
}

func (t *task) Cancel() {
	t.t.Log("Cannel")
}

func TestSubmit(t *testing.T) {
	timer, err := NewHashedWheelTimer()
	if err != nil {
		t.Fatal(err)
	}
	go timer.Start()
	defer timer.Stop()

	atask := &task{t: t, startTime: time.Now(), after: time.Second * 3}
	t.Logf("start: %s", time.Now().Format("2006/01/02 15:04:05"))
	timeout, err := timer.Submit(time.Second*3, atask)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("task id: %d", timeout.ID())
	time.Sleep(time.Second * 5)
	t.Logf("task status: %d", timeout.Status())
}

func TestCancel(t *testing.T) {
	timer, err := NewHashedWheelTimer()
	if err != nil {
		t.Fatal(err)
	}
	go timer.Start()
	defer timer.Stop()

	atask := &task{t: t, needCancel: true, startTime: time.Now(), after: time.Second * 5}
	t.Logf("start: %s", time.Now().Format("2006/01/02 15:04:05"))
	timeout, err := timer.Submit(time.Second*5, atask)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("task id: %d", timeout.ID())
	time.Sleep(time.Second * 2)
	timer.Cancel(timeout)
	time.Sleep(time.Second * 7)
	t.Logf("task status: %d", timeout.Status())
}

func TestSubmitMuch(t *testing.T) {
	timer, err := NewHashedWheelTimer()
	if err != nil {
		t.Fatal(err)
	}
	go timer.Start()
	defer timer.Stop()

	rand.New(rand.NewSource(time.Now().UnixNano()))
	var max time.Duration
	for i := 0; i < 100; i++ {
		after := time.Second * time.Duration(rand.Intn(200))
		task := &task{t: t, needCancel: true, startTime: time.Now(), after: after}
		if after > max {
			max = after
		}
		timer.Submit(after, task)
	}

	time.Sleep(max + time.Second)
}
