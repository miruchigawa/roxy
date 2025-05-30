package context

import (
	"strings"
	"sync"
	"time"

	"github.com/miruchigawa/roxy/options"
	"github.com/miruchigawa/roxy/types"
	"github.com/puzpuzpuz/xsync"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	waTypes "go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

type (
	MiddlewareFunc func(c *Ctx) bool
	RunFunc        func(c *Ctx) Result
	Result         *waE2E.Message
)

var contextPool sync.Pool

func init() {
	contextPool = sync.Pool{
		New: func() any {
			return &Ctx{}
		},
	}
}

func AcquireCtx() *Ctx {
	return contextPool.Get().(*Ctx)
}

func ReleaseCtx(c *Ctx) {
	contextPool.Put(c)
}

func NewCtx(locals *xsync.MapOf[string, string]) *Ctx {
	ctx := AcquireCtx()
	ctx.lastID = ""
	ctx.locals = locals
	ctx.argumentsMap = make(map[string]any)
	return ctx
}

type Ctx struct {
	client *whatsmeow.Client
	event  *events.Message
	info   waTypes.MessageInfo
	lastID waTypes.MessageID

	fromMe       bool
	number       string
	prefix       string
	parsedMsg    string
	arguments    []string
	argumentsMap map[string]any

	message   *waE2E.Message
	logger    waLog.Logger
	clientJid waTypes.JID
	options   *options.Options

	senderJid *waTypes.JID
	chatJid   *waTypes.JID

	locals        *xsync.MapOf[string, string]
	questionChan  chan *QuestionState
	pollingChan   chan *PollingState
	clientMethods types.ClientMethods
}

func (context *Ctx) SetClient(client *whatsmeow.Client) {
	context.client = client
}

func (context *Ctx) Client() *whatsmeow.Client {
	return context.client
}

func (context *Ctx) SetMessageEvent(evt *events.Message) {
	context.info = evt.Info
	context.fromMe = evt.Info.IsFromMe
	context.number = evt.Info.Sender.ToNonAD().String()
	context.message = evt.Message
	context.event = evt
}

func (context *Ctx) MessageEvent() *events.Message {
	return context.event
}

func (context *Ctx) MessageInfo() waTypes.MessageInfo {
	if context.event == nil {
		return waTypes.MessageInfo{}
	}
	return context.event.Info
}

func (context *Ctx) Message() *waE2E.Message {
	return context.message
}

func (context *Ctx) FromMe() bool {
	return context.fromMe
}

func (context *Ctx) Number() string {
	return context.number
}

func (context *Ctx) SetPrefix(prefix string) {
	context.prefix = prefix
}

func (context *Ctx) Prefix() string {
	return context.prefix
}

func (context *Ctx) Arguments() []string {
	return context.arguments
}

func (context *Ctx) ArgumentsShift() {
	context.arguments = context.arguments[1:]
}

func (context *Ctx) StoreArgument(name string, value any) {
	context.argumentsMap[name] = value
}

func (context *Ctx) GetArgumentString(key string) string {
	if v, ok := context.argumentsMap[key].(string); ok {
		return v
	} else {
		return ""
	}
}

func (context *Ctx) GetArgumentInt(key string) int64 {
	if v, ok := context.argumentsMap[key].(int64); ok {
		return v
	} else {
		return 0
	}
}

func (context *Ctx) GetArgumentFloat(key string) float64 {
	if v, ok := context.argumentsMap[key].(float64); ok {
		return v
	} else {
		return 0.0
	}
}

func (context *Ctx) GetArgumentBool(key string) bool {
	if v, ok := context.argumentsMap[key].(bool); ok {
		return v
	} else {
		return false
	}
}

func (context *Ctx) SetParsedMsg(parsedMsg string) {
	context.arguments = strings.Split(parsedMsg, " ")[1:]
	context.parsedMsg = parsedMsg
}

func (context *Ctx) ParsedMsg() string {
	return context.parsedMsg
}

func (context *Ctx) SetLogger(logger waLog.Logger) {
	context.logger = logger
}

func (context *Ctx) Logger() waLog.Logger {
	return context.logger
}

func (context *Ctx) SetClientJID(jid waTypes.JID) {
	context.clientJid = jid
}

func (context *Ctx) ClientJID() waTypes.JID {
	return context.clientJid
}

func (context *Ctx) SetOptions(options *options.Options) {
	context.options = options
}

func (context *Ctx) Options() *options.Options {
	return context.options
}

func (context *Ctx) SenderJID() waTypes.JID {
	return context.info.Sender.ToNonAD()
}

func (context *Ctx) ChatJID() waTypes.JID {
	return context.info.Chat.ToNonAD()
}

func (context *Ctx) SetClientMethods(clientMethods types.ClientMethods) {
	context.clientMethods = clientMethods
}

func (context *Ctx) Methods() types.ClientMethods {
	return context.clientMethods
}

func (context *Ctx) SetQuestionChan(question chan *QuestionState) {
	context.questionChan = question
}

func (context *Ctx) SetPollingChan(pooling chan *PollingState) {
	context.pollingChan = pooling
}

func (context *Ctx) RangeLocals(fun func(key string, value string) bool) {
	context.locals.Range(fun)
}

func (context *Ctx) GetLocals(key string) (string, bool) {
	return context.locals.Load(key)
}

func (context *Ctx) SetLocals(key string, value string) {
	context.locals.Store(key, value)
}

func (context *Ctx) DelLocals(key string) {
	context.locals.Delete(key)
}

func (context *Ctx) SetLocalsWithTTL(key string, value string, ttl time.Duration) {
	context.locals.Store(key, value)
	go func() {
		timeout := time.NewTimer(ttl)
		<-timeout.C
		context.locals.Delete(key)
		timeout.Stop()
	}()
}
