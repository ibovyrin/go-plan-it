package calendar

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	gCalendar "google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
	"log"
	"net/url"
	"os"
)

type Calendar struct {
	config     *oauth2.Config
	webhookUrl string
}

func NewCalendar(oauth2ConfigFile string) (*Calendar, error) {
	var webhookUrl = os.Getenv("WEBHOOK_URL")

	if webhookUrl == "" {
		return nil, fmt.Errorf("WEBHOOK_URL env variable is not set")

	}

	fileBytes, err := os.ReadFile(oauth2ConfigFile)
	if err != nil {
		log.Fatalf("Failed to read desktop.json: %v", err)
	}

	config, err := google.ConfigFromJSON(fileBytes, gCalendar.CalendarScope)
	if err != nil {
		return nil, fmt.Errorf("failed to parse oauth2ConfigFile: %w", err)
	}

	return &Calendar{
		config:     config,
		webhookUrl: webhookUrl,
	}, nil
}

func (c *Calendar) createService(ctx context.Context, token *oauth2.Token) (*gCalendar.Service, error) {
	client := c.config.Client(ctx, token)
	return gCalendar.NewService(ctx, option.WithHTTPClient(client))
}

func (c *Calendar) GetEventByID(ctx context.Context, eventId string, calendarId string, token *oauth2.Token) (*gCalendar.Event, error) {
	service, err := c.createService(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("GetEventByID: failed to create calendar service: %w", err)
	}

	e, err := service.Events.Get(calendarId, eventId).Do()
	if err != nil {
		return nil, fmt.Errorf("GetEventByID: failed to fetch calendar events: %w", err)
	}

	return e, nil
}

func (c *Calendar) GetCalendarsList(ctx context.Context, token *oauth2.Token) ([]*gCalendar.CalendarListEntry, error) {
	service, err := c.createService(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("GetCalendars: failed to create calendar service: %w", err)
	}

	response := make([]*gCalendar.CalendarListEntry, 0)

	pageToken := ""

	for {
		call := service.CalendarList.List()
		if pageToken != "" {
			call.PageToken(pageToken)
		}

		r, err := call.Do()
		if err != nil {
			return nil, fmt.Errorf("GetCalendars: failed to fetch calendar list: %w", err)
		}

		response = append(response, r.Items...)
		pageToken = r.NextPageToken
		if pageToken == "" {
			break
		}
	}

	return response, nil
}

func (c *Calendar) GetEventsList(ctx context.Context, calendarId, start, end string, maxResults int64, token *oauth2.Token) ([]*gCalendar.Event, error) {
	service, err := c.createService(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("GetEventsList: failed to create calendar service: %w", err)
	}

	response := make([]*gCalendar.Event, 0)
	pageToken := ""

	for {
		call := service.Events.List(calendarId).ShowDeleted(false).SingleEvents(true).TimeMin(start).TimeMax(end).MaxResults(maxResults).OrderBy("startTime")
		if pageToken != "" {
			call.PageToken(pageToken)
		}

		r, err := call.Do()
		if err != nil {
			return nil, fmt.Errorf("GetEventsList: failed to fetch calendar events: %w", err)
		}

		response = append(response, r.Items...)
		pageToken = r.NextPageToken
		if pageToken == "" {
			break
		}
	}

	return response, nil
}

func (c *Calendar) CreateEvent(ctx context.Context, calendarId string, event *gCalendar.Event, token *oauth2.Token) error {
	service, err := c.createService(ctx, token)
	if err != nil {
		return fmt.Errorf("CreateEvent: failed to create calendar service: %w", err)
	}

	call, err := service.Events.Insert(calendarId, event).Do()

	if err != nil {
		return fmt.Errorf("CreateEvent: failed to create task: %w", err)
	}
	event.HtmlLink = call.HtmlLink

	return nil
}

func (c *Calendar) CreateWatchChannel(ctx context.Context, calendarId, webhookPath string, token *oauth2.Token) (*gCalendar.Channel, error) {
	service, err := c.createService(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("CreateWatchChannel: failed to create calendar service: %w", err)
	}

	u, err := url.JoinPath(c.webhookUrl, webhookPath)
	if err != nil {
		return nil, fmt.Errorf("CreateWatchChannel: failed to create webhook url: %w", err)
	}

	channel := &gCalendar.Channel{
		Address: u,
		Id:      uuid.New().String(),
		Type:    "web_hook",
	}

	response, err := service.Events.Watch(calendarId, channel).Do()
	if err != nil {
		return nil, fmt.Errorf("CreateWatchChannel: failed to watch calendar: %w", err)
	}

	return response, nil
}

func (c *Calendar) DeleteWatchChannel(ctx context.Context, channelId, resourceId string, token *oauth2.Token) error {
	service, err := c.createService(ctx, token)
	if err != nil {
		return fmt.Errorf("DeleteWatchChannel: failed to create calendar service: %w", err)
	}

	err = service.Channels.Stop(&gCalendar.Channel{
		Id:         channelId,
		ResourceId: resourceId,
	}).Do()

	if err != nil {
		return fmt.Errorf("DeleteWatchChannel: failed to stop watching calendar: %w", err)
	}

	return nil
}

func (c *Calendar) GetAuthURL(state string) string {
	return c.config.AuthCodeURL(state, oauth2.AccessTypeOffline)
}

func (c *Calendar) ExchangeCode(ctx context.Context, code string) (*oauth2.Token, error) {
	token, err := c.config.Exchange(ctx, code, oauth2.AccessTypeOffline)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange token: %w", err)
	}

	return token, nil
}
