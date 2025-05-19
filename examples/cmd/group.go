package cmd

import (
	"fmt"
	"time"

	roxy "git.hanaworks.site/miruchigawa/roxy"
	"git.hanaworks.site/miruchigawa/roxy/context"
)

func init() {
	speed := roxy.NewCommand("group")
	speed.SetDescription("Testing latency")
	speed.UseCache(false)
	speed.SetGroupOnly(true)
	speed.SetRunFunc(groupFn)

	childNya := roxy.NewCommand("nya")
	childNya.SetDescription("Testing subcommand")
	childNya.Use(func(ctx *context.Ctx) bool {
		return true
	})
	childNya.SetRunFunc(nyaFn)

	speed.AddSubCommands(childNya)

	roxy.Commands.Add(speed)
}

func groupFn(ctx *context.Ctx) context.Result {
	nyow := time.Now()
	ctx.SendReplyMessage("Checking your connection speed~! (づ｡◕‿‿◕｡)づ")
	lawtency := time.Since(nyow).Milliseconds()
	return ctx.GenerateReplyMessage(fmt.Sprintf("Pong! Current connection latency is %d ms (づ｡◕‿‿◕｡)づ", lawtency))
}
