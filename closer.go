package closer

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

var (
	ExitWithSignalCode = false // if true, the singal handler exits with the caught signal code rather than ExitCodeErr

	ExitCodeOk  = 0 // the exit code used when there were no errors returned
	ExitCodeErr = 1 // the exit code used when one or more of the defered returns an error

	// DefaultSignals are the default signals handled by closer, you may append or change them to your liking.
	// note that once .Defer, .Init or .Exit are called, changing them doesn't change anything.
	DefaultSignals = []os.Signal{
		syscall.SIGINT,
		syscall.SIGHUP,
		syscall.SIGTERM,
	}

	OnError func(err error)
)

type closerFunc struct {
	fn func() error
}

func (cf *closerFunc) exec() (err error) {
	if cf.fn == nil {
		return
	}
	defer func() {
		if p := recover(); p != nil {
			if perr, ok := p.(error); ok {
				err = perr
			} else {
				err = fmt.Errorf("panic: %v", p)
			}
		}
	}()
	err, cf.fn = cf.fn(), nil
	return
}

type closerFuncs []closerFunc

func (cfs closerFuncs) cleanup() bool {
	var errored bool
	for i := len(cfs) - 1; i > -1; i-- {
		if err := cfs[i].exec(); err != nil {
			errored = true
			if OnError != nil {
				OnError(err)
			}
		}
	}
	return errored
}

type closer struct {
	sync.Mutex
	sync.Once
	sigCh   chan os.Signal
	closers closerFuncs
}

func (c *closer) waitForSignal() {
	for sig := range c.sigCh {
		c.Lock()
		c.closers.cleanup()
		c.Unlock()
		if sig, ok := sig.(syscall.Signal); ok && ExitWithSignalCode {
			os.Exit(int(sig))
		} else {
			os.Exit(ExitCodeErr)
		}
	}
}

func (c *closer) deferFuncs(fns ...interface{}) func() {
	cfs := make(closerFuncs, len(fns))
	for i, fn := range fns {
		cfn := &cfs[i]
		switch fn := fn.(type) {
		case func():
			cfn.fn = func() error { fn(); return nil }
		case func() error:
			cfn.fn = fn
		case io.Closer:
			cfn.fn = fn.Close
		default:
			panic("supported closers: func(), func() error and io.Closer")
		}
	}
	c.Lock()
	c.closers = append(c.closers, cfs...)
	c.Unlock()
	return func() {
		c.Lock()
		cfs.cleanup()
		c.Unlock()
	}
}

func (c *closer) reinit(force bool, signals ...os.Signal) {
	if len(signals) == 0 {
		signals = DefaultSignals
	}
	c.Lock()
	defer c.Unlock()
	if c.sigCh == nil {
		c.sigCh = make(chan os.Signal, 1)
		go c.waitForSignal()
	} else if !force {
		return
	} else {
		signal.Stop(c.sigCh)
	}
	signal.Notify(c.sigCh, signals...)

}

var gC closer

func get() *closer {
	gC.Do(func() { gC.reinit(false) })
	return &gC
}

// SetSignals intalizes the global closer with the provided signals,
// if len(signals) == 0, it uses the default signals.
// If SetSignals is never called, it will
func SetSignals(signals ...os.Signal) {
	gC.reinit(true, signals...)
}

// Defer ensures all the functions passed are executed in a LIFO order.
// Init(DefaultSignals) will be automatically called if the user didn't manually call it.
// fns can be either func(), func() error or an io.Closer.
// returns a func() that triggers all the passed funcs.
// example:
// 	defer closer.Defer(mux.Unlock, f.Close)()
func Defer(fns ...interface{}) func() {
	return get().deferFuncs(fns...)
}

// Exit calls all the defered funcs and calls os.Exit
// if code == -1, then its set to ExitCodeErr or ExitCodeOk depending on if there were any errors returned.
func Exit(code int) {
	c := get()
	c.Lock()
	erred := c.closers.cleanup()
	c.Unlock()
	if code != -1 {
		os.Exit(code)
	}
	if erred {
		os.Exit(ExitCodeErr)
	} else {
		os.Exit(ExitCodeOk)
	}
}
