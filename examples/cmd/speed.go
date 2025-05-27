package cmd

import (
	"fmt"
	"time"

	roxy "git.hanaworks.site/miruchigawa/roxy"
	"git.hanaworks.site/miruchigawa/roxy/context"
)

func init() {
	speed := roxy.NewCommand("speed")
	speed.SetDescription("Testing latency")
	speed.UseCache(false)
	speed.SetRunFunc(speedFn)

	childNya := roxy.NewCommand("nya")
	childNya.SetDescription("Testing subcommand")
	childNya.AddArgument("test", "Testo", roxy.ArgumentBool, false, roxy.ArgumentOptions{DefaultValue: true})
	childNya.SetRunFunc(nyaFn)

	say := roxy.NewCommand("say")
	say.SetDescription("Say something")
	say.AddArgument("message", "Message to say", roxy.ArgumentString, true, roxy.ArgumentOptions{IsCatchAll: true})
	say.SetRunFunc(sayFn)

	speed.AddSubCommands(childNya)
	speed.AddSubCommands(say)

	roxy.Commands.Add(speed)
}

func speedFn(ctx *context.Ctx) context.Result {
	nyow := time.Now()
	ctx.SendReplyMessage("Checking your connection speed~! (づ｡◕‿‿◕｡)づ")
	lawtency := time.Since(nyow).Milliseconds()
	return ctx.GenerateReplyMessage(fmt.Sprintf("Pong! Current connection latency is %d ms (づ｡◕‿‿◕｡)づ", lawtency))
}

func nyaFn(ctx *context.Ctx) context.Result {
	if ctx.GetArgumentBool("test") {
		return ctx.GenerateReplyMessage("Nya!")
	} else {
		return ctx.GenerateReplyMessage("Nya? (｡•́︿•̀｡)")
	}
}

func sayFn(ctx *context.Ctx) context.Result {
	return ctx.GenerateReplyMessage("You said: " + ctx.GetArgumentString("message"))
}
