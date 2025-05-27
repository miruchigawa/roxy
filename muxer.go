package roxy

import (
	"bytes"
	gcontext "context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"git.hanaworks.site/miruchigawa/roxy/context"
	"git.hanaworks.site/miruchigawa/roxy/options"
	"git.hanaworks.site/miruchigawa/roxy/types"
	"git.hanaworks.site/miruchigawa/roxy/util"
	"github.com/google/uuid"
	"github.com/puzpuzpuz/xsync"
	"github.com/sajari/fuzzy"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	waProto "go.mau.fi/whatsmeow/proto/waE2E"
	waTypes "go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

type Muxer struct {
	Options              *options.Options                             `json:"options,omitempty"`
	Log                  waLog.Logger                                 `json:"log,omitempty"`
	MessageTimeout       time.Duration                                `json:"message_timeout,omitempty"`
	Categories           *xsync.MapOf[string, string]                 `json:"categories,omitempty"`
	GlobalMiddlewares    *xsync.MapOf[string, context.MiddlewareFunc] `json:"global_middlewares,omitempty"`
	Middlewares          *xsync.MapOf[string, context.MiddlewareFunc] `json:"middlewares,omitempty"`
	Commands             *xsync.MapOf[string, *Command]               `json:"commands,omitempty"`
	CommandResponseCache *xsync.MapOf[string, *waProto.Message]       `json:"command_response_cache,omitempty"`
	QuestionState        *xsync.MapOf[string, *context.QuestionState] `json:"question_state,omitempty"`
	PollingState         *xsync.MapOf[string, *context.PollingState]  `json:"polling_state,omitempty"`
	GroupCache           *xsync.MapOf[string, []*waTypes.GroupInfo]   `json:"group_cache,omitempty"`
	Locals               *xsync.MapOf[string, string]                 `json:"locals,omitempty"`

	QuestionChan    chan *context.QuestionState                           `json:"question_chan,omitempty"`
	PollingChan     chan *context.PollingState                            `json:"polling_chan,omitempty"`
	SuggestionModel *fuzzy.Model                                          `json:"suggestion_model,omitempty"`
	CommandParser   func(str string) (prefix string, cmd string, ok bool) `json:"command_parser,omitempty"`

	types.AppMethods
}

func NewMuxer(log waLog.Logger, options *options.Options, appMethods types.AppMethods) *Muxer {
	muxer := &Muxer{
		Locals:               xsync.NewMapOf[string](),
		Commands:             xsync.NewMapOf[*Command](),
		GlobalMiddlewares:    xsync.NewMapOf[context.MiddlewareFunc](),
		Middlewares:          xsync.NewMapOf[context.MiddlewareFunc](),
		CommandResponseCache: xsync.NewMapOf[*waProto.Message](),
		QuestionState:        xsync.NewMapOf[*context.QuestionState](),
		PollingState:         xsync.NewMapOf[*context.PollingState](),
		Categories:           xsync.NewMapOf[string](),
		GroupCache:           xsync.NewMapOf[[]*waTypes.GroupInfo](),
		QuestionChan:         make(chan *context.QuestionState),
		PollingChan:          make(chan *context.PollingState),
		MessageTimeout:       options.SendMessageTimeout,
		Options:              options,
		Log:                  log,
		CommandParser:        util.ParseCmd,
		AppMethods:           appMethods,
	}

	go muxer.handleQuestionStateChannel()
	go muxer.handlePollingStateChannel()

	return muxer
}

func (muxer *Muxer) AddCommandParser(context func(str string) (prefix string, cmd string, ok bool)) {
	muxer.CommandParser = context
}

func (muxer *Muxer) Clean() {
	muxer.Categories.Range(func(key string, category string) bool {
		muxer.Categories.Delete(key)
		return true
	})
	muxer.Commands.Range(func(key string, cmd *Command) bool {
		muxer.Commands.Delete(key)
		return true
	})
	muxer.Middlewares.Range(func(key string, middleware context.MiddlewareFunc) bool {
		muxer.Middlewares.Delete(key)
		return true
	})
	muxer.Locals.Range(func(key string, middleware string) bool {
		muxer.Locals.Delete(key)
		return true
	})
}

func (muxer *Muxer) handlePollingStateChannel() {
	for message := range muxer.PollingChan {
		muxer.PollingState.Store(message.PollId, message)
		if message.PollingTimeout != nil {
			go func() {
				timeout := time.NewTimer(*message.PollingTimeout)
				<-timeout.C
				message.ResultChan <- true
				timeout.Stop()
				muxer.PollingState.Delete(message.PollId)
			}()
		} else {
			go func() {
				timeout := time.NewTimer(time.Minute * 10)
				<-timeout.C
				message.ResultChan <- true
				timeout.Stop()
				muxer.PollingState.Delete(message.PollId)
			}()
		}
	}
}

func (muxer *Muxer) handleQuestionStateChannel() {
	for message := range muxer.QuestionChan {
		muxer.QuestionState.Delete(message.Ctx.Number())
		for _, question := range message.Questions {
			if question.GetAnswer() == "" {
				message.ActiveQuestion = question.Question
				muxer.QuestionState.Store(message.Ctx.Number(), message)
				if question.Question != "" {
					message.Ctx.SendReplyMessage(question.Question)
				}
				break
			}
		}
	}
}

func (muxer *Muxer) addEmbedCommands() {
	categories := Categories.Get()
	for _, cat := range categories {
		muxer.Categories.Store(cat, cat)
	}
	commands := Commands.Get()
	for _, cmd := range commands {
		muxer.AddCommand(cmd)
	}
	middlewares := Middlewares.Get()
	for _, mid := range middlewares {
		muxer.AddMiddleware(mid)
	}
	globalMiddleware := GlobalMiddlewares.Get()
	for _, mid := range globalMiddleware {
		muxer.AddGlobalMiddleware(mid)
	}

	if muxer.Options.CommandSuggestion {
		muxer.GenerateSuggestionModel()
	}
}

func (muxer *Muxer) AddGlobalMiddleware(middleware context.MiddlewareFunc) {
	muxer.GlobalMiddlewares.Store(uuid.New().String(), middleware)
}

func (muxer *Muxer) AddMiddleware(middleware context.MiddlewareFunc) {
	muxer.Middlewares.Store(uuid.New().String(), middleware)
}

func (muxer *Muxer) AddCommand(cmd *Command) {
	cmd.Validate()
	_, ok := muxer.Commands.Load(cmd.Name)
	if ok {
		panic("error: duplicate command " + cmd.Name)
	}

	for _, alias := range cmd.Aliases {
		_, ok := muxer.Commands.Load(alias)
		if ok {
			panic("error: duplicate alias in command " + cmd.Name)
		}
		muxer.Commands.Store(alias, cmd)
	}
	muxer.Commands.Store(cmd.Name, cmd)
}

func (muxer *Muxer) GetActiveCommand() []*Command {
	cmd := make([]*Command, 0)
	muxer.Commands.Range(func(key string, value *Command) bool {
		// filter alias commands
		if key == value.Name {
			cmd = append(cmd, value)
		}
		return true
	})

	return cmd
}

func (muxer *Muxer) GetActiveGlobalMiddleware() []context.MiddlewareFunc {
	middleware := make([]context.MiddlewareFunc, 0)
	muxer.GlobalMiddlewares.Range(func(key string, value context.MiddlewareFunc) bool {
		middleware = append(middleware, value)
		return true
	})

	return middleware
}

func (muxer *Muxer) GetActiveMiddleware() []context.MiddlewareFunc {
	middleware := make([]context.MiddlewareFunc, 0)
	muxer.Middlewares.Range(func(key string, value context.MiddlewareFunc) bool {
		middleware = append(middleware, value)
		return true
	})

	return middleware
}

func (muxer *Muxer) getCachedCommandResponse(cmd string) *waProto.Message {
	cache, ok := muxer.CommandResponseCache.Load(cmd)
	if ok {
		return cache
	}
	return nil
}

func (muxer *Muxer) setCacheCommandResponse(cmd string, response *waProto.Message) {
	muxer.CommandResponseCache.Store(cmd, response)
	timeout := time.NewTimer(muxer.Options.CommandResponseCacheTimeout)
	<-timeout.C
	muxer.CommandResponseCache.Delete(cmd)
	timeout.Stop()
}

func (muxer *Muxer) globalMiddlewareProcessing(c *whatsmeow.Client, evt *events.Message) bool {
	if muxer.GlobalMiddlewares.Size() >= 1 {
		ctx := context.NewCtx(muxer.Locals)
		ctx.SetClient(c)
		ctx.SetLogger(muxer.Log)
		ctx.SetOptions(muxer.Options)
		ctx.SetMessageEvent(evt)
		ctx.SetClientJID(muxer.ClientJID())
		ctx.SetClientMethods(muxer)
		ctx.SetQuestionChan(muxer.QuestionChan)
		ctx.SetPollingChan(muxer.PollingChan)
		defer context.ReleaseCtx(ctx)

		midAreOk := true
		muxer.GlobalMiddlewares.Range(func(key string, value context.MiddlewareFunc) bool {
			if !value(ctx) {
				midAreOk = false
				return false
			}
			return true
		})
		return midAreOk
	}

	return true
}

func (muxer *Muxer) handlePollingState(c *whatsmeow.Client, evt *events.Message) {
	if evt.Message.PollUpdateMessage.PollCreationMessageKey == nil && evt.Message.PollUpdateMessage.PollCreationMessageKey.ID == nil {
		return
	}

	pollingState, ok := muxer.PollingState.Load(*evt.Message.PollUpdateMessage.PollCreationMessageKey.ID)
	if ok {
		pollMessage, err := c.DecryptPollVote(gcontext.Background(), evt)
		if err != nil {
			return
		}

		var result []string
		for _, selectedOption := range pollMessage.SelectedOptions {
			for _, option := range pollingState.PollOptions {
				if bytes.Equal(selectedOption, option.Hashed) {
					result = append(result, option.Options)
					break
				}
			}
		}

		pollingState.PollingResult = append(pollingState.PollingResult, result...)
		if pollingState.PollingTimeout == nil {
			pollingState.ResultChan <- true
			muxer.PollingState.Delete(pollingState.PollId)
		}
	}
}

func (muxer *Muxer) handleQuestionState(c *whatsmeow.Client, evt *events.Message, number, parsedMsg string) {
	questionState, _ := muxer.QuestionState.Load(number)
	if strings.Contains(parsedMsg, "cancel") || strings.Contains(parsedMsg, "batal") {
		muxer.QuestionState.Delete(number)
		return
	} else {
		if questionState.WithEmojiReact {
			muxer.SendEmojiMessage(evt, questionState.EmojiReact)
		}

		jids := []waTypes.MessageID{
			evt.Info.ID,
		}
		c.MarkRead(jids, evt.Info.Timestamp, evt.Info.Chat, evt.Info.Sender)

		for i, question := range questionState.Questions {
			if question.Question == questionState.ActiveQuestion && question.GetAnswer() == "" {
				if questionState.Questions[i].Capture {
					questionState.Questions[i].SetAnswer(evt.Message)
				} else if questionState.Questions[i].Reply {
					result := util.GetQuotedText(evt)
					questionState.Questions[i].SetAnswer(result)
				} else {
					questionState.Questions[i].SetAnswer(parsedMsg)
				}
				continue
			} else if question.Question != questionState.ActiveQuestion && question.GetAnswer() == "" {
				questionState.ActiveQuestion = question.Question
				if question.Question != "" {
					questionState.Ctx.SendReplyMessage(question.Question)
				}
				return
			} else if question.Question == questionState.ActiveQuestion && question.GetAnswer() != "" {
				continue
			}
		}

		muxer.QuestionState.Delete(number)
		questionState.ResultChan <- true
		return
	}
}

func (muxer *Muxer) RunCommand(c *whatsmeow.Client, evt *events.Message) {
	if !muxer.isAllowedSource(evt) || !muxer.isFromValidSender(evt) || evt.Info.ID == "status@broadcast" {
		return
	}

	if poll := evt.Message.GetPollUpdateMessage(); poll != nil {
		muxer.handlePollingState(c, evt)
		return
	}

	sender := evt.Info.Sender.ToNonAD().String()
	parsed := util.ParseMessageText(evt)

	if _, ok := muxer.QuestionState.Load(sender); ok {
		muxer.handleQuestionState(c, evt, sender, parsed)
		return
	}

	if !muxer.globalMiddlewareProcessing(c, evt) {
		return
	}

	prefix, name, isCmd := muxer.CommandParser(parsed)
	command, exists := muxer.Commands.Load(name)

	switch {
	case isCmd && !exists && muxer.Options.CommandSuggestion:
		muxer.SuggestCommand(evt, prefix, name)
	case isCmd && exists:
		muxer.execute(c, evt, sender, parsed, prefix, command)
	}
}

func (muxer *Muxer) execute(
	client *whatsmeow.Client,
	evt *events.Message,
	sender, parsed, prefix string,
	command *Command,
) {
	ctx := context.NewCtx(muxer.Locals)
	defer context.ReleaseCtx(ctx)
	ctx.SetClient(client)
	ctx.SetLogger(muxer.Log)
	ctx.SetOptions(muxer.Options)
	ctx.SetMessageEvent(evt)
	ctx.SetClientJID(muxer.ClientJID())
	ctx.SetParsedMsg(parsed)
	ctx.SetPrefix(prefix)
	ctx.SetClientMethods(muxer)
	ctx.SetQuestionChan(muxer.QuestionChan)
	ctx.SetPollingChan(muxer.PollingChan)

	var key string
	if child := muxer.resolveSubCommand(command, ctx); child != nil {
		key = fmt.Sprintf("%s-%s", command.Name, child.Name)
		command = child
		if !muxer.guardsPass(evt, command) || !muxer.runAllMiddlewares(ctx, command) {
			return
		}
	} else {
		if !muxer.guardsPass(evt, command) || !muxer.runAllMiddlewares(ctx, command) {
			return
		}
		key = command.Name
	}

	go muxer.markAsReadAndLogCommand(client, evt, sender, key)

	arguments, err := ParseCommandArguments(ctx.Arguments(), command.Arguments)
	if err != nil {
		ctx.SendReplyMessage(fmt.Sprintf("Error: %s", err.Error()))
		return
	}

	for k, v := range arguments {
		ctx.StoreArgument(k, v)
	}

	var message *waProto.Message
	if command.Cache {
		if cached := muxer.getCachedCommandResponse(key); cached != nil {
			message = cached
		} else {
			message = command.RunFunc(ctx)
			if message != nil {
				go muxer.setCacheCommandResponse(key, message)
			}
		}
	} else {
		message = command.RunFunc(ctx)
	}

	if message != nil {
		muxer.SendMessage(evt.Info.Chat, message)
	}
}

func (muxer *Muxer) resolveSubCommand(command *Command, ctx *context.Ctx) *Command {
	if len(command.SubCommands) <= 0 || len(ctx.Arguments()) <= 0 {
		return nil
	}

	if child, ok := command.SubCommands[ctx.Arguments()[0]]; ok {
		ctx.ArgumentsShift()
		return child
	}

	return nil
}

func (muxer *Muxer) guardsPass(evt *events.Message, command *Command) bool {
	if command.GroupOnly && !evt.Info.IsGroup || command.PrivateOnly && evt.Info.IsGroup {
		return false
	}

	if command.OnlyAdminGroup && evt.Info.IsGroup {
		ok, _ := muxer.IsGroupAdmin(evt.Info.Chat, evt.Info.Sender)
		if !ok {
			return false
		}
	}

	if command.OnlyIfBotAdmin && evt.Info.IsGroup {
		ok, _ := muxer.IsClientGroupAdmin(evt.Info.Chat)
		if !ok {
			return false
		}
	}

	return true
}

func (muxer *Muxer) runAllMiddlewares(ctx *context.Ctx, cmd *Command) bool {
	var wg sync.WaitGroup
	passed := true

	muxer.Middlewares.Range(func(_ string, mw context.MiddlewareFunc) bool {
		wg.Add(1)
		if !mw(ctx) {
			passed = false
			wg.Done()
			return false
		}
		wg.Done()
		return true
	})
	wg.Wait()

	if !passed {
		return false
	}

	if cmd.Middleware != nil && !cmd.Middleware(ctx) {
		return false
	}
	return true
}

func (muxer *Muxer) isAllowedSource(evt *events.Message) bool {
	switch {
	case muxer.Options.AllowFromPrivate && !muxer.Options.AllowFromGroup && evt.Info.IsGroup:
		return false
	case muxer.Options.AllowFromGroup && !muxer.Options.AllowFromPrivate && !evt.Info.IsGroup:
		return false
	case !muxer.Options.AllowFromGroup && !muxer.Options.AllowFromPrivate:
		return false
	}
	return true
}

func (muxer *Muxer) isFromValidSender(evt *events.Message) bool {
	return !muxer.Options.OnlyFromSelf || evt.Info.IsFromMe
}

func (muxer *Muxer) markAsReadAndLogCommand(c *whatsmeow.Client, evt *events.Message, number string, cmd string) {
	if muxer.Options.WithCommandLog {
		muxer.Log.Infof("[CMD] [%s] command > %s", number, cmd)
	}
	if err := c.MarkRead([]waTypes.MessageID{evt.Info.ID}, evt.Info.Timestamp, evt.Info.Chat, evt.Info.Sender); err != nil {
		muxer.Log.Errorf("read message error: %v", err)
	}
}

// func (muxer *Muxer) safeGo(fn func()) {
// 	go func() {
// 		defer func() {
// 			if r := recover(); r != nil {
// 				muxer.Log.Errorf("spawn panic: %v", r)
// 			}
// 		}()
// 		fn()
// 	}()
// }

func (muxer *Muxer) SendEmojiMessage(event *events.Message, emoji string) {
	id := event.Info.ID
	chat := event.Info.Chat
	sender := event.Info.Sender
	key := &waCommon.MessageKey{
		FromMe:    proto.Bool(true),
		ID:        proto.String(id),
		RemoteJID: proto.String(chat.String()),
	}

	if !sender.IsEmpty() && sender.User != muxer.AppMethods.ClientJID().ToNonAD().String() {
		key.FromMe = proto.Bool(false)
		key.Participant = proto.String(sender.ToNonAD().String())
	}

	message := &waProto.Message{
		ReactionMessage: &waProto.ReactionMessage{
			Key:               key,
			Text:              proto.String(emoji),
			SenderTimestampMS: proto.Int64(time.Now().UnixMilli()),
		},
	}

	muxer.SendMessage(event.Info.Chat, message)
}

func ParseCommandArguments(argsList []string, argDefinitions map[string]Argument) (map[string]any, error) {
	parsedArgs := make(map[string]any)

	canonicalKeyMap := make(map[string]string)
	for keyName := range argDefinitions {
		canonicalKeyMap[strings.ToLower(keyName)] = keyName
	}

	rawUserArgs := make(map[string]string)
	var currentCanonicalKey string
	var currentValueParts []string
	consumedTokenIndices := make(map[int]bool)

	for i, token := range argsList {
		isKey := false

		if strings.HasSuffix(token, ":") {
			potentialKeyNameLower := strings.ToLower(strings.TrimSuffix(token, ":"))
			if canonicalName, ok := canonicalKeyMap[potentialKeyNameLower]; ok {
				if currentCanonicalKey != "" && len(currentValueParts) > 0 {
					rawUserArgs[currentCanonicalKey] = strings.TrimSpace(strings.Join(currentValueParts, " "))
				}
				currentCanonicalKey = canonicalName
				currentValueParts = []string{}
				isKey = true
				consumedTokenIndices[i] = true
			}
		}

		if !isKey {
			if currentCanonicalKey != "" {
				currentValueParts = append(currentValueParts, token)
				consumedTokenIndices[i] = true
			}
		}
	}

	if currentCanonicalKey != "" && len(currentValueParts) > 0 {
		rawUserArgs[currentCanonicalKey] = strings.TrimSpace(strings.Join(currentValueParts, " "))
	}

	var unconsumedTokens []string
	for i, token := range argsList {
		if !consumedTokenIndices[i] {
			unconsumedTokens = append(unconsumedTokens, token)
		}
	}

	var catchAllArgDef *Argument
	var catchAllArgCanonicalName string

	for _, def := range argDefinitions {
		localDef := def
		if localDef.IsCatchAll {
			if catchAllArgDef != nil {
				return nil, fmt.Errorf("only one catch-all argument is allowed, found multiple: '%s' and '%s'", catchAllArgCanonicalName, localDef.Name)
			}
			if localDef.Type != ArgumentString {
				return nil, fmt.Errorf("catch-all argument '%s' must be of type string", localDef.Name)
			}

			catchAllArgDef = &localDef
			catchAllArgCanonicalName = localDef.Name
		}
	}

	if len(unconsumedTokens) > 0 {
		if catchAllArgDef != nil {
			if _, alreadySetExplicitly := rawUserArgs[catchAllArgCanonicalName]; alreadySetExplicitly {
				return nil, fmt.Errorf("catch-all argument '%s' was already set explicitly", catchAllArgCanonicalName)
			}
			rawUserArgs[catchAllArgCanonicalName] = strings.Join(unconsumedTokens, " ")
		} else {
			return nil, fmt.Errorf("unconsumed tokens found: %s", strings.Join(unconsumedTokens, " "))
		}
	}

	for _, argDef := range argDefinitions {
		canonicalName := argDef.Name
		userProvidedValue, wasProvidedByUser := rawUserArgs[canonicalName]

		if wasProvidedByUser {
			var convertedValue any
			var conversionError error

			if userProvidedValue == "" && argDef.Type != ArgumentString {
				if argDef.Required || argDef.DefaultValue == nil {
					return nil, fmt.Errorf("empty argument was found '%s' with type %s",
						canonicalName, argTypeToString(argDef.Type))
				}
			} else {
				switch argDef.Type {
				case ArgumentString:
					convertedValue = userProvidedValue
				case ArgumentInt:
					convertedValue, conversionError = strconv.Atoi(userProvidedValue)
				case ArgumentFloat:
					convertedValue, conversionError = strconv.ParseFloat(userProvidedValue, 64)
				case ArgumentBool:
					lowerVal := strings.ToLower(userProvidedValue)
					if lowerVal == "true" || lowerVal == "t" || lowerVal == "1" {
						convertedValue = true
					} else if lowerVal == "false" || lowerVal == "f" || lowerVal == "0" {
						convertedValue = false
					} else {
						conversionError = fmt.Errorf("'%s' not a boolean value (true/false/t/f/1/0)", userProvidedValue)
					}
				}

				if conversionError != nil {
					return nil, fmt.Errorf("invalid argument '%s': expected %s, found '%s'",
						canonicalName, argTypeToString(argDef.Type), userProvidedValue)
				}
				parsedArgs[canonicalName] = convertedValue
			}
		}

		if _, stillNotProcessed := parsedArgs[canonicalName]; !stillNotProcessed {
			if !wasProvidedByUser || (userProvidedValue == "" && argDef.Type != ArgumentString && argDef.DefaultValue != nil) {
				if argDef.Required && !wasProvidedByUser {
					return nil, fmt.Errorf("Argument %s is required", canonicalName)
				}
				if argDef.DefaultValue != nil {
					parsedArgs[canonicalName] = argDef.DefaultValue
				} else if argDef.Required {
					return nil, fmt.Errorf("Argument %s is required", canonicalName)
				}
			}
		}
	}

	return parsedArgs, nil
}

func argTypeToString(argType ArgumentType) string {
	switch argType {
	case ArgumentString:
		return "string"
	case ArgumentInt:
		return "integer"
	case ArgumentFloat:
		return "float"
	case ArgumentBool:
		return "boolean"
	default:
		return "unknown"
	}
}
