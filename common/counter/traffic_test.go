package counter

import (
	"bytes"
	"net"
	"sync"
	"testing"
)

type memoryConn struct {
	net.Conn
	input  *bytes.Reader
	output bytes.Buffer
}

func (c *memoryConn) Read(p []byte) (int, error)  { return c.input.Read(p) }
func (c *memoryConn) Write(p []byte) (int, error) { return c.output.Write(p) }

func TestConnCounterAccumulatesReturnedBytes(t *testing.T) {
	storage := &TrafficStorage{}
	conn := NewConnCounter(&memoryConn{input: bytes.NewReader([]byte("abcdefg"))}, storage)
	for i := 0; i < 4; i++ {
		_, _ = conn.Read(make([]byte, 2))
	}
	_, _ = conn.Write([]byte("abc"))
	_, _ = conn.Write([]byte("defg"))
	up, down := storage.Snapshot(false)
	if up != 7 || down != 7 {
		t.Fatalf("got up=%d down=%d; want 7 each", up, down)
	}
}

func TestSnapshotPreservesConcurrentIncrements(t *testing.T) {
	storage := &TrafficStorage{}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10000; j++ {
				storage.UpCounter.Add(1)
				storage.DownCounter.Add(2)
			}
		}()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	var up, down int64
	for {
		u, d := storage.Snapshot(true)
		up += u
		down += d
		select {
		case <-done:
			u, d = storage.Snapshot(true)
			up += u
			down += d
			if up != 40000 || down != 80000 {
				t.Fatalf("lost bytes: %d/%d", up, down)
			}
			return
		default:
		}
	}
}
