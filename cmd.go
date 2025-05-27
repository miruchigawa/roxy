package roxy

import (
	"fmt"
	"sort"

	"git.hanaworks.site/miruchigawa/roxy/context"
	"git.hanaworks.site/miruchigawa/roxy/types"
)

var (
	Commands          types.Embed[*Command]
	GlobalMiddlewares types.Embed[context.MiddlewareFunc]
	Middlewares       types.Embed[context.MiddlewareFunc]
	Categories        types.Embed[string]
)

func init() {
	cmd := types.NewEmbed[*Command]()
	mid := types.NewEmbed[context.MiddlewareFunc]()
	cat := types.NewEmbed[string]()
	gMid := types.NewEmbed[context.MiddlewareFunc]()

	Commands = &cmd
	Middlewares = &mid
	Categories = &cat
	GlobalMiddlewares = &gMid
}

type Command struct {
	Name        string
	Aliases     []string
	Description string

	Category string
	Cache    bool

	HideFromHelp bool
	GroupOnly    bool
	PrivateOnly  bool

	OnlyAdminGroup   bool
	OnlyIfBotAdmin   bool
	AdditionalValues map[string]any

	SubCommands map[string]*Command

	Arguments  map[string]Argument
	RunFunc    context.RunFunc
	Middleware context.MiddlewareFunc
}

type ArgumentType int

const (
	ArgumentInt ArgumentType = iota
	ArgumentFloat
	ArgumentString
	ArgumentBool
)

type Argument struct {
	Name         string
	Type         ArgumentType
	Description  string
	Required     bool
	DefaultValue any
	IsCatchAll   bool
}

type ArgumentOptions struct {
	DefaultValue any
	IsCatchAll   bool
}

func NewCommand(name string) *Command {
	return &Command{
		Name:             name,
		Description:      fmt.Sprintf("This is %s command description example", name),
		SubCommands:      make(map[string]*Command),
		AdditionalValues: make(map[string]any),
		Arguments:        make(map[string]Argument),
	}
}

func (c *Command) SetDescription(value string) {
	c.Description = value
}

func (c *Command) SetAliases(values []string) {
	c.Aliases = values
}

func (c *Command) SetCategory(value string) {
	c.Category = value
}

func (c *Command) UseCache(value bool) {
	c.Cache = value
}

func (c *Command) SetHideFromHelp(value bool) {
	c.HideFromHelp = value
}

func (c *Command) SetGroupOnly(value bool) {
	c.GroupOnly = value
}

func (c *Command) SetPrivateOnly(value bool) {
	c.PrivateOnly = value
}

func (c *Command) SetOnlyAdminGroup(value bool) {
	c.OnlyAdminGroup = value
}

func (c *Command) SetOnlyIfBotAdmin(value bool) {
	c.OnlyIfBotAdmin = value
}

func (c *Command) SetAdditionalValues(value map[string]any) {
	c.AdditionalValues = value
}

func (c *Command) AddSubCommands(cmd *Command) {
	c.SubCommands[cmd.Name] = cmd
}

func (c *Command) SetRunFunc(fn context.RunFunc) {
	c.RunFunc = fn
}

func (c *Command) AddArgument(name, description string, argType ArgumentType, required bool, options ...ArgumentOptions) {
	var opt ArgumentOptions

	if len(options) > 0 {
		opt = options[0]
	}

	c.Arguments[name] = Argument{
		Name:         name,
		Description:  description,
		Type:         argType,
		Required:     required,
		DefaultValue: opt.DefaultValue,
		IsCatchAll:   opt.IsCatchAll,
	}
}

func (c *Command) Use(fn context.MiddlewareFunc) {
	c.Middleware = fn
}

func (c *Command) Validate() {
	if c.Name == "" {
		panic("error: command name cannot be empty")
	}
	if c.Description == "" {
		c.Description = fmt.Sprintf("This is %s command description example", c.Name)
	}
	if c.PrivateOnly && c.GroupOnly {
		panic("error: invalid scope group/private?")
	}

	for _, child := range c.SubCommands {
		if len(child.SubCommands) > 0 {
			panic("error: subcommands can't have children")
		}
	}

	sort.Strings(c.Aliases)
}
