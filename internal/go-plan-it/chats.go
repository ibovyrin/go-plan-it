package go_plan_it

import (
	"fmt"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
	"time"
)

type Chat struct {
	ChatId     int64 `gorm:"primaryKey;autoIncrement:false"`
	Registered bool

	ChannelId         *string
	ChannelExpiration *int64
	ChannelResourceId *string
	CalendarId        *string
	NextUpdateAt      *int64
	NextEventId       *string
	Token             *oauth2.Token `gorm:"serializer:json"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

type Chats struct {
	db *gorm.DB
}

func NewChats(db *gorm.DB) *Chats {
	chats := Chats{
		db: db,
	}
	return &chats
}

func (c *Chats) CreateChat(id int64) (*Chat, error) {
	chat := Chat{
		ChatId:     id,
		Registered: false,
	}

	if err := c.db.Create(&chat).Error; err != nil {
		return nil, fmt.Errorf("CreateChat: failed to create chat: %w", err)
	}

	return &chat, nil
}

func (c *Chats) GetChatById(id int64) (*Chat, error) {
	var chat Chat
	if err := c.db.First(&chat, id).Error; err != nil {
		return nil, fmt.Errorf("GetChatById: failed to get chat: %w", err)
	}

	return &chat, nil
}

func (c *Chats) DeleteChatById(id int64) error {
	if err := c.db.Delete(&Chat{}, id).Error; err != nil {
		return fmt.Errorf("DeleteChatById: failed to delete chat: %w", err)
	}

	return nil
}

func (c *Chats) UpdateChat(chat *Chat) error {
	if err := c.db.Where(Chat{ChatId: chat.ChatId}).Save(chat).Error; err != nil {
		return fmt.Errorf("UpdateChat: failed to save chat: %w", err)
	}
	return nil
}

func (c *Chats) GetActiveChats() ([]*Chat, error) {
	chats := make([]*Chat, 0)
	if err := c.db.Where("registered = ? AND calendar_id IS NOT NULL", true).Find(&chats).Error; err != nil {
		return nil, fmt.Errorf("GetActiveChats: failed to get chat: %w", err)
	}
	return chats, nil
}
