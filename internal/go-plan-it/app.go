package go_plan_it

import (
	"context"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/golang-module/carbon"
	"github.com/ibovyrin/go-plan-it/pkg/calendar"
	"github.com/ibovyrin/go-plan-it/pkg/gpt"
	"github.com/ibovyrin/go-plan-it/pkg/tgbot"
	gCalendar "google.golang.org/api/calendar/v3"
	"gorm.io/gorm"
	"log/slog"
	"strconv"
	"strings"
)

const errorMessage = "Something went wrong. Please try again later."

var specialChars = []string{
	"\\", "_", "*", "[", "]", "(", ")", "~", "`", ">",
	"&", "#", "-", "=", "|", "{", "}", ".", "!"}

type App struct {
	chats    *Chats
	gpt      *gpt.GPT
	calendar *calendar.Calendar
	bot      *tgbot.Bot
	logger   *slog.Logger
}

func NewApp(chats *Chats, gpt *gpt.GPT, calendar *calendar.Calendar, logger *slog.Logger, bot *tgbot.Bot) (*App, error) {
	app := App{
		chats:    chats,
		gpt:      gpt,
		calendar: calendar,
		bot:      bot,
		logger:   logger.WithGroup("app"),
	}

	return &app, nil
}

func (a *App) HandleWatchCommand(c *tgbot.Context) {
	l := a.logger.With("chat_id", c.ChatId, "command", "/watch")

	chat, err := a.chats.GetChatById(c.ChatId)
	if err != nil {
		l.Error(fmt.Sprintf("Failed to get chat: %s", err))
		c.AbortWithMessage(errorMessage)
		return
	}

	if c.Update.Message.CommandArguments() == "" {
		calendars, err := a.calendar.GetCalendarsList(context.Background(), chat.Token)
		if err != nil {
			l.Error(fmt.Sprintf("Failed to get calendars list: %s", err))
			c.AbortWithMessage(errorMessage)
			return
		}

		c.AddMessage("Please select a calendar you want to watch:")
		for _, cld := range calendars {
			c.AddMessageWithOptions(fmt.Sprintf("/watch %s %s", cld.Summary, cld.Id), tgbot.MessageWithOptions{DisableWebPagePreview: true})
		}
		c.AddMessage("Just copy and paste one of these options.")
		return
	}

	ctx := context.Background()
	args := strings.Split(c.Update.Message.CommandArguments(), " ")

	chat.CalendarId = &args[1]
	webhookPath := strconv.FormatInt(c.ChatId, 10)
	channel, err := a.calendar.CreateWatchChannel(ctx, *chat.CalendarId, webhookPath, chat.Token)
	if err != nil {
		l.Error(fmt.Sprintf("Failed to create watch channel: %s", err))
		return
	}

	chat.ChannelId = &channel.Id
	chat.ChannelResourceId = &channel.ResourceId
	chat.ChannelExpiration = &channel.Expiration

	err = a.setNextUpdateTimeForChat(ctx, chat)
	if err != nil {
		l.Error(fmt.Sprintf("Failed to set next chat update time: %s", err))
		c.AbortWithMessage(errorMessage)
	}

	c.AddMessage("Calendar successfully set. You will receive notifications about upcoming events.")
}

func (a *App) HandleStopWatchCommand(c *tgbot.Context) {
	l := a.logger.With("chat_id", c.ChatId, "command", "/stopwatch")

	chat, err := a.chats.GetChatById(c.ChatId)
	if err != nil {
		l.Error(fmt.Sprintf("Failed to get chat: %s", err))
		c.AbortWithMessage(errorMessage)
		return
	}

	err = a.calendar.DeleteWatchChannel(context.Background(), *chat.ChannelId, *chat.ChannelResourceId, chat.Token)
	if err != nil {
		l.Error(fmt.Sprintf("Failed to delete watch channel: %s", err))
		c.AbortWithMessage(errorMessage)
		return
	}

	chat.CalendarId = nil
	chat.NextEventId = nil
	chat.NextUpdateAt = nil
	chat.ChannelId = nil
	chat.ChannelResourceId = nil
	chat.ChannelExpiration = nil

	if err := a.chats.UpdateChat(chat); err != nil {
		l.Error(fmt.Sprintf("Failed to update chat: %s", err))
		c.AbortWithMessage(errorMessage)
		return
	}

	c.AddMessage("You have successfully unsubscribed from calendar events.")
}

func (a *App) HandleStartCommand(c *tgbot.Context) {
	l := a.logger.With("chat_id", c.ChatId, "command", "/start")

	chat, err := a.chats.CreateChat(c.ChatId)

	switch {
	case errors.Is(err, gorm.ErrDuplicatedKey):
		c.AbortWithMessage("You are already registered. Use /stop to stop using this bot.")
		return
	case !errors.Is(err, nil):
		l.Error(fmt.Sprintf("Failed to create chat: %s", err))
		c.AbortWithMessage(errorMessage)
		return
	}

	if chat.Registered {
		c.AbortWithMessage("You are already registered. Use /stop to stop using this bot.")
		return
	}

	c.AddMessage("Hello, I'm a bot that can show you your tasks from Google Calendar.\nIf you want to use me, you need to authorize me.\n")
	state := strconv.FormatInt(c.ChatId, 10)
	authCodeURL := a.calendar.GetAuthURL(state)
	c.AddMessage(fmt.Sprintf("In order to authorize me, follow this link: \n%s", authCodeURL))
}

func (a *App) getChatById(c *tgbot.Context) (*Chat, error) {
	chat, err := a.chats.GetChatById(c.ChatId)
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		c.AbortWithMessage("You are not registered. Use /start to start using this bot.")
		return nil, err
	case !errors.Is(err, nil):
		c.AbortWithMessage(errorMessage)
		return nil, err
	}

	return chat, nil
}

func (a *App) IsExists(c *tgbot.Context) {
	l := a.logger.With("chat_id", c.ChatId, "middleware", "IsRegistered")

	_, err := a.getChatById(c)
	if err != nil {
		l.Error(fmt.Sprintf("Failed to get chat: %s", err))
		return
	}
}

func (a *App) IsRegistered(c *tgbot.Context) {
	l := a.logger.With("chat_id", c.ChatId, "middleware", "IsRegistered")

	chat, err := a.getChatById(c)
	if err != nil {
		l.Error(fmt.Sprintf("Failed to get chat: %s", err))
		return
	}

	if !chat.Registered {
		c.AbortWithMessage("You are not registered. Use /start to start using this bot.")
		return
	}
}

func (a *App) IsSubscribed(c *tgbot.Context) {
	l := a.logger.With("chat_id", c.ChatId, "middleware", "IsSubscribed")

	chat, err := a.getChatById(c)
	if err != nil {
		l.Error(fmt.Sprintf("Failed to get chat: %s", err))
		return
	}

	if !chat.Registered {
		c.AbortWithMessage("You are not registered. Use /start to start using this bot.")
		return
	}

	if chat.CalendarId == nil {
		c.AbortWithMessage("You are not subscribed to any calendar. Use /watch to subscribe.")
		return
	}
}

func (a *App) HandleStopCommand(c *tgbot.Context) {
	l := a.logger.With("chat_id", c.ChatId, "command", "/stop")

	err := a.chats.DeleteChatById(c.ChatId)
	if err != nil {
		l.Error(fmt.Sprintf("Failed to delete chat: %s", err))
		c.AbortWithMessage(errorMessage)
		return
	}

	c.AddMessage("You have successfully unsubscribed from this bot.")
}

func (a *App) HandleEventsCommand(c *tgbot.Context) {
	l := a.logger.With("chat_id", c.ChatId, "command", "/events")

	chat, err := a.chats.GetChatById(c.ChatId)
	if err != nil {
		l.Error(fmt.Sprintf("Failed to get chat: %s", err))
		c.AbortWithMessage(errorMessage)
		return
	}

	ctx := context.Background()
	start := carbon.Now().SubWeeks(2).ToRfc3339String()
	end := carbon.Now().AddWeeks(1).ToRfc3339String()

	eventsList, err := a.calendar.GetEventsList(ctx, *chat.CalendarId, start, end, 100, chat.Token)
	if err != nil {
		l.Error(fmt.Sprintf("Failed to get events list: %s", err))
		c.AbortWithMessage(errorMessage)
		return
	}

	if len(eventsList) == 0 {
		c.AbortWithMessage("You have no upcoming events.")
		return
	}

	c.AddMessage("Here are your upcoming events:")
	for _, event := range eventsList {
		c.AddMessageWithOptions(a.EventToString(event), tgbot.MessageWithOptions{
			ParseMode:             tgbotapi.ModeMarkdownV2,
			DisableWebPagePreview: true,
		})
	}
}

func (a *App) HandleNewEventCommand(c *tgbot.Context) {
	l := a.logger.With("chat_id", c.ChatId, "command", "/new")

	_, err := a.chats.GetChatById(c.ChatId)
	if err != nil {
		l.Error(fmt.Sprintf("Failed to get chat: %s", err))
		c.AbortWithMessage(errorMessage)
		return
	}

	c.AddMessage("What is the task?")
	c.RegisterWaitForInput()
}

func (a *App) HandleNewEventCommandResponse(c *tgbot.Context) {
	l := a.logger.With("chat_id", c.ChatId, "command", "/new_response")

	chat, err := a.chats.GetChatById(c.ChatId)
	if err != nil {
		l.Error(fmt.Sprintf("Failed to get chat: %s", err))
		c.AbortWithMessage(errorMessage)
		return
	}

	text := c.Update.Message.Text
	if text == "" {
		c.AbortWithMessage("You need to specify the task and date. Please start again /new.")
		return
	}

	req := gpt.Request{
		Description: text,
		Today:       carbon.Now().String(),
	}

	resp, err := a.gpt.ParseRequest(&req)
	if err != nil {
		l.Error(fmt.Sprintf("Failed to parse request with gpt: %s", err))
		c.AbortWithMessage(errorMessage)
		return
	}

	start := carbon.Parse(resp.Date)
	end := start.AddMinutes(30)
	ctx := context.Background()

	e := &gCalendar.Event{
		// TODO add support for attachments
		//Attachments:               nil,
		Description: resp.Notes,
		Start:       &gCalendar.EventDateTime{DateTime: start.ToRfc3339String()},
		End:         &gCalendar.EventDateTime{DateTime: end.ToRfc3339String()},
		Summary:     resp.Title,
	}

	err = a.calendar.CreateEvent(ctx, *chat.CalendarId, e, chat.Token)
	if err != nil {
		l.Error(fmt.Sprintf("Failed create a new event: %s", err))
		c.AbortWithMessage(errorMessage)
		return
	}

	c.AddMessageWithOptions(fmt.Sprintf("I created an event:\n%s", a.EventToString(e)), tgbot.MessageWithOptions{
		ParseMode:             tgbotapi.ModeMarkdownV2,
		DisableWebPagePreview: true,
	})

	err = a.setNextUpdateTimeForChat(ctx, chat)
	if err != nil {
		l.Error(fmt.Sprintf("Failed setNextUpdateTimeForChat: %s", err))
		return
	}
}

func (a *App) SendMorningAgenda(c *tgbot.Context) {
	l := a.logger.With("scheduled", "SendMorningAgenda")

	chats, err := a.chats.GetActiveChats()
	if err != nil {
		l.Error(fmt.Sprintf("Failed get active chats: %s", err))
		return
	}

	for _, chat := range chats {
		ctx := context.Background()
		start := carbon.Now().StartOfDay().ToRfc3339String()
		end := carbon.Now().EndOfDay().ToRfc3339String()

		eventsList, err := a.calendar.GetEventsList(ctx, *chat.CalendarId, start, end, 100, chat.Token)
		if err != nil {
			l.Error(fmt.Sprintf("Failed to get events list: %s", err))
			return
		}

		if len(eventsList) == 0 {
			c.AddMessageConfig(tgbot.CreateMessage(chat.ChatId, "You don't have any tasks for today."))
			c.Abort()
			return
		}

		c.AddMessageConfig(tgbot.CreateMessage(chat.ChatId, "Here is your list for today:"))

		for _, e := range eventsList {
			msg := tgbot.CreateMessageWithOptions(chat.ChatId, a.EventToString(e), tgbot.MessageWithOptions{
				ParseMode:             tgbotapi.ModeMarkdownV2,
				DisableWebPagePreview: true,
			})
			c.AddMessageConfig(msg)
		}
	}
}

func (a *App) SendNotifications(c *tgbot.Context) {
	l := a.logger.With("scheduled", "SendNotifications")
	l.Debug("running")

	chats, err := a.chats.GetActiveChats()
	if err != nil {
		l.Error(fmt.Sprintf("Failed get active chats: %s", err))
		return
	}

	l.Debug(fmt.Sprintf("running notifications for chats %d", len(chats)))

	for _, chat := range chats {
		if chat.NextUpdateAt != nil && *chat.NextUpdateAt > carbon.Now().Timestamp() {
			continue
		}

		ctx := context.Background()
		if chat.NextEventId == nil {
			err = a.setNextUpdateTimeForChat(ctx, chat)
			if err != nil {
				l.Error(fmt.Sprintf("Failed to set next chat update time: %s", err))
			}
			continue
		}

		e, err := a.calendar.GetEventByID(ctx, *chat.NextEventId, *chat.CalendarId, chat.Token)
		if err != nil {
			l.Error(fmt.Sprintf("Failed to get event by id: %s", err))
			return
		}

		msg := tgbot.CreateMessageWithOptions(chat.ChatId, fmt.Sprintf("You have a task:\n%s", a.EventToString(e)), tgbot.MessageWithOptions{
			ParseMode:             tgbotapi.ModeMarkdownV2,
			DisableWebPagePreview: true,
		})
		c.AddMessageConfig(msg)

		err = a.setNextUpdateTimeForChat(ctx, chat)
		if err != nil {
			l.Error(fmt.Sprintf("Failed to set next chat update time: %s", err))
		}

		if *chat.ChannelExpiration < carbon.Now().Timestamp() {
			webhookPath := strconv.FormatInt(c.ChatId, 10)
			channel, err := a.calendar.CreateWatchChannel(ctx, *chat.CalendarId, webhookPath, chat.Token)
			if err != nil {
				l.Error(fmt.Sprintf("Failed to create channel: %s", err))
				msg := tgbot.CreateMessage(chat.ChatId, "Something wrong with update channels...")
				c.AddMessageConfig(msg)
				continue
			}

			err = a.calendar.DeleteWatchChannel(ctx, *chat.CalendarId, *chat.ChannelResourceId, chat.Token)
			if err != nil {
				l.Error(fmt.Sprintf("Failed to delete channel: %s", err))
				msg := tgbot.CreateMessage(chat.ChatId, "Something wrong with update channels...")
				c.AddMessageConfig(msg)
				continue
			}

			chat.ChannelId = &channel.Id
			chat.ChannelResourceId = &channel.ResourceId

			err = a.chats.UpdateChat(chat)
			if err != nil {
				l.Error(fmt.Sprintf("Failed to update chat while renew channel: %s", err))
				msg := tgbot.CreateMessage(chat.ChatId, "Something wrong with update channels...")
				c.AddMessageConfig(msg)
				continue
			}

		}
	}
}

func (a *App) HandleCalendarWebhook(c *gin.Context) {
	c.JSON(200, gin.H{"message": "success"})

	chatId := c.Param("chatId")
	channelId := c.Request.Header.Get("X-Goog-Channel-ID")
	channelExpiration := c.Request.Header.Get("X-Goog-Channel-Expiration")
	resourceId := c.Request.Header.Get("X-Goog-Resource-ID")
	l := a.logger.With("http", "HandleCalendarWebhook",
		"chatId",
		chatId,
		"channelId",
		channelId,
		"channelExpiration",
		channelExpiration,
		"resourceId",
		resourceId)

	l.Info("event update webhook triggered")

	id, err := strconv.ParseInt(chatId, 10, 64)
	if err != nil {
		l.Error(fmt.Sprintf("Failed to parse chatId: %v", err))
		return
	}
	chat, err := a.chats.GetChatById(id)
	if err != nil {
		l.Error(fmt.Sprintf("Failed to get chat by id: %v", err))
		return
	}

	if chat == nil || chat.CalendarId == nil || chat.Token == nil {
		l.Error(fmt.Sprintf("chat with id is not configured: %d", chat.ChatId))
		return
	}

	err = a.setNextUpdateTimeForChat(context.Background(), chat)
	if err != nil {
		l.Error(fmt.Sprintf("Failed to setNextUpdateTimeForChat: %v", err))
		return
	}
}

func (a *App) HandleLoginWebhook(c *gin.Context) {
	c.JSON(200, gin.H{"message": "success"})

	chatId := c.Query("state")

	l := a.logger.With("http", "HandleLoginWebhook",
		"chatId",
		chatId)
	l.Info("login webhook triggered")

	id, err := strconv.ParseInt(chatId, 10, 64)
	if err != nil {
		l.Error(fmt.Sprintf("Failed to parse chatId: %v", err))
		return
	}
	chat, err := a.chats.GetChatById(id)
	if err != nil {
		l.Error(fmt.Sprintf("Failed to get chat by id: %v", err))
		return
	}

	token, err := a.calendar.ExchangeCode(context.Background(), c.Query("code"))
	if err != nil {
		l.Error(fmt.Sprintf("Failed ExchangeCode: %v", err))
		return
	}

	chat.Token = token
	chat.Registered = true
	err = a.chats.UpdateChat(chat)
	if err != nil {
		l.Error(fmt.Sprintf("Failed UpdateChat: %v", err))
		return
	}
	a.bot.SendMessages([]*tgbotapi.MessageConfig{tgbot.CreateMessage(chat.ChatId, "You successfully authenticated! Please use /watch command to subscribe to a calendar.")})
}

func (a *App) EventToString(event *gCalendar.Event) string {
	date := event.Start.DateTime
	if date == "" {
		date = event.Start.Date
	}

	summary := event.Summary

	for _, c := range specialChars {
		summary = strings.Replace(summary, c, fmt.Sprintf("\\%s", c), -1)
	}

	return fmt.Sprintf("[%s](%s) \\- %s", summary, event.HtmlLink, carbon.Parse(date).DiffForHumans())
}

func (a *App) setNextUpdateTimeForChat(ctx context.Context, chat *Chat) error {
	start := carbon.Now()
	end := carbon.Now().AddDays(1)

	eventsList, err := a.calendar.GetEventsList(ctx, *chat.CalendarId, start.ToRfc3339String(), end.ToRfc3339String(), 10, chat.Token)
	if err != nil {
		return fmt.Errorf("failed to get events list: %w", err)
	}

	t := carbon.Now().AddHours(1).Timestamp()
	chat.NextEventId = nil

	for _, e := range eventsList {
		eventStartTime := carbon.Parse(e.Start.DateTime).Timestamp()
		if eventStartTime > start.Timestamp() && t > eventStartTime {
			t = eventStartTime
			chat.NextEventId = &e.Id
		}
	}

	chat.NextUpdateAt = &t
	if err := a.chats.UpdateChat(chat); err != nil {
		return fmt.Errorf("failed to update chat: %w", err)
	}

	return nil
}
