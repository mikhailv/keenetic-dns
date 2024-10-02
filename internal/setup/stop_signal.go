package setup

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func ListenStopSignal(parentCtx context.Context) context.Context {
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(parentCtx)
	go func() {
		<-signalCh
		cancel()
	}()
	return ctx
}
