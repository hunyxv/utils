package timer

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/panjf2000/ants/v2"
)

func TestTimerWheel(t *testing.T) {
	timer, err := NewHashedWheelTimer(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	go timer.Start()
	defer timer.Stop()

	t.Log("start time: ", time.Now())
	tt := timer.Submit(3*time.Minute, func() {
		t.Log("timer task ", time.Now())
	})
	//time.Sleep(time.Second)
	//tt.Reset()
	t.Log(tt.TID())

	time.Sleep(3*time.Minute + 200*time.Millisecond)
}

func TestTimerCancel(t *testing.T) {
	timer, err := NewHashedWheelTimer(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	go timer.Start()
	defer timer.Stop()

	t.Log("start time: ", time.Now())
	tt := timer.Submit(3*time.Second, func() {
		t.Log("timer task ", time.Now())
	})
	time.Sleep(time.Second)
	tt.Cancel()

	time.Sleep(4*time.Second + 100*time.Millisecond)
}

func TestTimerWheel0(t *testing.T) {
	timer, err := NewHashedWheelTimer(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	go timer.Start()
	defer timer.Stop()

	t.Log("start time: ", time.Now())
	tt := timer.Submit(0, func() {
		t.Log("timer task ", time.Now())
	})
	time.Sleep(time.Second)
	t.Log(tt.TID())

	time.Sleep(4*time.Second + 100*time.Millisecond)
}

func TestTimerN(t *testing.T) {
	gpool, _ := ants.NewPool(100000, ants.WithNonblocking(true))
	timer, err := NewHashedWheelTimer(context.Background(), WithWorkPool(gpool))
	if err != nil {
		t.Fatal(err)
	}
	go timer.Start()
	defer timer.Stop()

	m := map[uint64]TimerTask{}

	tasks := []TimerTask{}
	now := time.Now()
	for i := 0; i < 100000; i++ {
		n := rand.Intn(20000)
		task := timer.Submit(time.Duration(n)*time.Millisecond, func() {
			//	t.Logf("[%s]: %dms", time.Now(), n)
		})
		tasks = append(tasks, task)
		m[task.TID()] = task
	}

	for {
		for i := 0; i < len(tasks); {
			task := tasks[i]
			if task.Status() == Done {
				tasks[i] = tasks[len(tasks)-1]
				tasks[len(tasks)-1] = nil
				tasks = tasks[:len(tasks)-1]
				continue
			}
			i++
		}
		if len(tasks) == 0 {
			t.Log("End: ", time.Since(now))
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	for _, task := range m {
		if task.ExecutionTime().Sub(task.ExpirationTime()) > time.Second {
			t.Log(task.ExecutionTime().Sub(task.ExpirationTime()))
		}
	}

}
