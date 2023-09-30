package tgbot

import (
	"fmt"
	"github.com/go-co-op/gocron"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"log/slog"
	"os"
	"slices"
	"strings"
)

type MessageWithOptions struct {
	ParseMode             string
	DisableWebPagePreview bool
}

type Context struct {
	ChatId           int64
	Command          string
	Update           tgbotapi.Update
	responseMessages []*tgbotapi.MessageConfig
	aborted          bool
	bot              *Bot
}

func (c *Context) Abort() {
	c.aborted = true
}

func (c *Context) AbortWithMessage(message string) {
	c.AddMessage(message)
	c.Abort()
}

func (c *Context) IsAborted() bool {
	return c.aborted
}

func (c *Context) RegisterWaitForInput() {
	command := c.bot.responseHandlerName(c.Command)
	c.bot.waitFor[c.ChatId] = WaitForCommand{
		UpdateID: c.Update.UpdateID + 1,
		Command:  command,
	}
}

func (c *Context) AddMessageConfig(msg *tgbotapi.MessageConfig) {
	c.responseMessages = append(c.responseMessages, msg)
}

func (c *Context) AddMessage(message string) {
	msg := CreateMessage(c.ChatId, message)
	c.responseMessages = append(c.responseMessages, msg)
}

func (c *Context) AddMessageWithOptions(message string, options MessageWithOptions) {
	msg := CreateMessageWithOptions(c.ChatId, message, options)
	c.responseMessages = append(c.responseMessages, msg)
}

func (c *Context) GetMessages() []*tgbotapi.MessageConfig {
	return c.responseMessages
}

type WaitForCommand struct {
	UpdateID int
	Command  string
}

type Bot struct {
	client         *tgbotapi.BotAPI
	waitFor        map[int64]WaitForCommand
	handlers       map[string][]func(*Context)
	updatesTimeout int
	scheduler      *gocron.Scheduler
	logger         *slog.Logger
	allowList      []string
}

func NewBot(updatesTimeout int, scheduler *gocron.Scheduler, logger ...*slog.Logger) (*Bot, error) {
	var tgBotToken = os.Getenv("TG_BOT_TOKEN")

	if tgBotToken == "" {
		return nil, fmt.Errorf("TG_BOT_TOKEN env variable is not set")

	}

	var allowList = os.Getenv("TG_BOT_ALLOW_LIST")

	client, err := tgbotapi.NewBotAPI(tgBotToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create new bot: %w", err)

	}

	client.Debug = true
	bot := &Bot{
		client:         client,
		waitFor:        make(map[int64]WaitForCommand),
		updatesTimeout: updatesTimeout,
		scheduler:      scheduler,
	}

	if allowList == "" {
		bot.allowList = make([]string, 0)
	} else {
		bot.allowList = strings.Split(allowList, ",")
	}

	if len(logger) > 0 {
		bot.logger = logger[0].WithGroup("tgbot")
	} else {
		bot.logger = slog.New(slog.NewTextHandler(os.Stdout, nil)).WithGroup("tgbot")
	}

	return bot, nil
}

func (b *Bot) responseHandlerName(command string) string {
	return fmt.Sprintf("%s_responseHandler", command)
}

func (b *Bot) RegisterCommand(command string, handlers []func(*Context), responseHandler ...bool) {
	if b.handlers == nil {
		b.handlers = make(map[string][]func(*Context))
	}

	if len(responseHandler) > 0 && responseHandler[0] {
		command = b.responseHandlerName(command)
	}

	b.handlers[command] = append(b.handlers[command], handlers...)
	b.logger.Debug("RegisterCommand: added handler", "command", command)
}

func (b *Bot) scheduledHandlerWrapper(handler func(*Context)) {
	context := Context{
		responseMessages: make([]*tgbotapi.MessageConfig, 0),
		aborted:          false,
	}
	handler(&context)
	b.SendMessages(context.GetMessages())
}

func (b *Bot) RegisterScheduledHandler(cron string, handler func(*Context)) error {
	if _, err := b.scheduler.CronWithSeconds(cron).Do(b.scheduledHandlerWrapper, handler); err != nil {
		return err
	}
	return nil
}

func (b *Bot) defaultHandler(context *Context) {
	context.AbortWithMessage("I don't know what to do with this message.")
}

func (b *Bot) getHandlers(chatId int64, updateId int, command string) []func(*Context) {
	wait, ok := b.waitFor[chatId]
	if ok {
		delete(b.waitFor, chatId)
	}
	b.logger.Debug(fmt.Sprintf("wait %v", wait))

	if wait.UpdateID == updateId && command == "" {
		command = wait.Command
	}

	handlers, ok := b.handlers[command]
	if !ok {
		handlers = []func(*Context){b.defaultHandler}
	}

	b.logger.Debug("getHandlers: discovered handlers", "handlers_num", len(handlers), "command", command)
	return handlers
}

func (b *Bot) SendMessages(messages []*tgbotapi.MessageConfig) {
	for _, message := range messages {
		if _, err := b.client.Send(message); err != nil {
			b.logger.Error("SendMessages: failed to send message", "error", err)
		}
	}
}

func (b *Bot) RunUpdatesHandler() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = b.updatesTimeout
	updates := b.client.GetUpdatesChan(u)

	b.scheduler.StartAsync()

	for update := range updates {
		if update.Message == nil {
			continue
		}

		b.logger.Debug("RunUpdatesHandler: received update",
			"update_id",
			update.UpdateID,
			"chat_id",
			update.Message.Chat.ID,
			"text",
			update.Message.Text,
			"command", update.Message.Command(),
			"user_name", update.Message.From.UserName,
		)

		if len(b.allowList) > 0 {
			if !slices.Contains(b.allowList, update.Message.From.UserName) {
				msg := CreateMessage(update.Message.Chat.ID, "Sorry, I can't talk to you.")
				b.SendMessages([]*tgbotapi.MessageConfig{msg})
				continue
			}
		}

		context := Context{
			ChatId:           update.Message.Chat.ID,
			Command:          update.Message.Command(),
			Update:           update,
			responseMessages: make([]*tgbotapi.MessageConfig, 0),
			aborted:          false,
			bot:              b,
		}

		for _, handler := range b.getHandlers(update.Message.Chat.ID, update.UpdateID, update.Message.Command()) {
			handler(&context)
			if context.IsAborted() {
				break
			}
		}

		b.SendMessages(context.GetMessages())
	}
}

func (b *Bot) StopUpdatesHandler() {
	b.scheduler.Stop()
	b.client.StopReceivingUpdates()
}

func CreateMessageWithOptions(chatId int64, text string, options ...MessageWithOptions) *tgbotapi.MessageConfig {
	msg := tgbotapi.NewMessage(chatId, text)
	if len(options) > 0 {
		msg.DisableWebPagePreview = options[0].DisableWebPagePreview
		msg.ParseMode = options[0].ParseMode
	}
	return &msg
}

func CreateMessage(chatId int64, text string) *tgbotapi.MessageConfig {
	return CreateMessageWithOptions(chatId, text)
}
