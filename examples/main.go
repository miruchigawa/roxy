package main

import (
	"os"
	"os/signal"
	"syscall"

	roxy "git.hanaworks.site/miruchigawa/roxy"
	_ "git.hanaworks.site/miruchigawa/roxy/examples/cmd"
	"git.hanaworks.site/miruchigawa/roxy/options"
)

func main() {
	opt := options.NewDefaultOptions()
	opt.HostNumber = os.Getenv("HOST_NUMBER")
	opt.LoginOptions = options.PAIR_CODE
	opt.HistorySync = true

	app, err := roxy.NewRoxyBase(opt)
	if err != nil {
		panic(err)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	app.Shutdown()
}
