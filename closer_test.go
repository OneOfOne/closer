package closer_test

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/OneOfOne/closer"
)

var testSignal = os.Getenv("TEST_SIGNAL") == "1"

func TestCloser(t *testing.T) {
	var vals []int
	fn := closer.Defer(func() { vals = append(vals, 1) }, func() { vals = append(vals, 0) })
	fn()
	fn()
	if len(vals) != 2 {
		t.Fatalf("expected 2 values, got %d", len(vals))
	}
	if vals[0] != 0 && vals[1] != 1 {
		t.Fatalf("unexpected output: %v", vals)
	}
}

func TestSignal(t *testing.T) {
	if testSignal {
		closer.ExitWithSignalCode = false
		defer closer.Defer(func() { closer.ExitCodeErr = 55 })()
		select {}
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestSignal")
	cmd.Env = append(os.Environ(), "TEST_SIGNAL=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(time.Millisecond * 10)
		cmd.Process.Signal(syscall.SIGTERM)
	}()
	if err := cmd.Wait(); err != nil {
		if !strings.Contains(err.Error(), "55") {
			t.Fatalf("unexpected exit code: %v", err)
		}
	} else {
		t.Fatal("process exited without error")
	}
}
