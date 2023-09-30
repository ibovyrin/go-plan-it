package main

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/go-co-op/gocron"
	goplanit "github.com/ibovyrin/go-plan-it/internal/go-plan-it"
	"github.com/ibovyrin/go-plan-it/pkg/calendar"
	"github.com/ibovyrin/go-plan-it/pkg/gpt"
	"github.com/ibovyrin/go-plan-it/pkg/tgbot"
	"log/slog"
	"os"
	"time"
)

const (
	oauth2ConfigFile = "credentials.json"
	notifications    = "*/5 * * * * *"
	morningUpdate    = "00 45 8 * * *"
	updatesTimeout   = 60
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)

	s := gocron.NewScheduler(time.Local)

	bot, err := tgbot.NewBot(updatesTimeout, s, logger)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to start app: %s", err))
		os.Exit(1)
	}

	g, err := gpt.NewGPT()
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to start app: %s", err))
		os.Exit(1)
	}

	c, err := calendar.NewCalendar(oauth2ConfigFile)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to start app: %s", err))
		os.Exit(1)
	}

	db, err := goplanit.NewDB()
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to start app: %s", err))
		os.Exit(1)
	}

	chats := goplanit.NewChats(db)

	app, err := goplanit.NewApp(chats, g, c, logger, bot)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to start app: %s", err))
		os.Exit(1)
	}

	err = bot.RegisterScheduledHandler(notifications, app.SendNotifications)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to start app: %s", err))
		os.Exit(1)
	}
	err = bot.RegisterScheduledHandler(morningUpdate, app.SendMorningAgenda)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to start app: %s", err))
		os.Exit(1)
	}

	bot.RegisterCommand("new", []func(*tgbot.Context){app.IsSubscribed, app.HandleNewEventCommand})
	bot.RegisterCommand("new", []func(*tgbot.Context){app.IsSubscribed, app.HandleNewEventCommandResponse}, true)

	bot.RegisterCommand("events", []func(*tgbot.Context){app.IsSubscribed, app.HandleEventsCommand})
	bot.RegisterCommand("watch", []func(*tgbot.Context){app.IsRegistered, app.HandleWatchCommand})
	bot.RegisterCommand("stopwatch", []func(*tgbot.Context){app.IsSubscribed, app.HandleStopWatchCommand})
	bot.RegisterCommand("start", []func(*tgbot.Context){app.HandleStartCommand})
	bot.RegisterCommand("stop", []func(*tgbot.Context){app.IsExists, app.HandleStopCommand})

	router := gin.Default()
	router.GET("/login", app.HandleLoginWebhook)
	router.POST("/webhook/:chatId", app.HandleCalendarWebhook)

	router.POST("/webhook", app.HandleCalendarWebhook)

	// TODO graceful shutdown
	go bot.RunUpdatesHandler()
	err = router.Run("0.0.0.0:80")
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to start app: %s", err))
		os.Exit(1)
	}
}
