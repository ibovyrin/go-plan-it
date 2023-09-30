package gpt

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/sashabaranov/go-openai"
	"os"
)

const systemPrompt = `You are integrated into a scheduling system.
Users will send you messages, requesting to schedule a task.
Messages will always be in JSON format and should contain two fields: description and today.
description: Contains the task's details.
today: Indicates today's date.

Your role is to analyze the request and respond with an object in valid JSON format.
This object should contain three fields: title, notes, and date.

title: The task's title.
notes: A summary of the task.
date: Extracted from the message, indicating when the task should be executed, in the same date format as received.

Instructions:
If the incoming message is in the wrong format, you must respond with the error: "wrong format".
You should Ensure that you correct any orthographical errors present in the message.
You must respond in the same language as the original message in the description field.
If a date is specified without a time, you should schedule the task for 09:30.

Your response should be swift and accurate to facilitate effective task scheduling.`

type GPT struct {
	client *openai.Client
}

type Response struct {
	Title string `json:"title"`
	Date  string `json:"date"`
	Notes string `json:"notes"`
}

type Request struct {
	Description string `json:"description"`
	Today       string `json:"today"`
}

var system = openai.ChatCompletionMessage{
	Role:    openai.ChatMessageRoleSystem,
	Content: systemPrompt,
}

func NewGPT() (*GPT, error) {
	var openAiToken = os.Getenv("OPENAI_TOKEN")

	if openAiToken == "" {
		return nil, fmt.Errorf("OPENAI_TOKEN env variable is not set")
	}

	client := openai.NewClient(openAiToken)

	_, err := client.ListEngines(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to create gpt client: %w", err)
	}
	return &GPT{client: client}, nil
}

func (c *GPT) ParseRequest(request *Request) (Response, error) {
	var response Response

	data, err := json.Marshal(request)
	if err != nil {
		return response, fmt.Errorf("ParseRequest error: %w", err)
	}

	resp, err := c.client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model:            openai.GPT4,
			Temperature:      1,
			MaxTokens:        256,
			TopP:             1,
			FrequencyPenalty: 0,
			PresencePenalty:  0,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: string(data),
				},
				system,
			},
		},
	)

	if err != nil {
		return response, fmt.Errorf("ParseRequest error: %w", err)
	}

	if len(resp.Choices) == 0 {
		return response, fmt.Errorf("ParseRequest error: no completion")
	}

	err = json.Unmarshal([]byte(resp.Choices[0].Message.Content), &response)
	if err != nil {
		return response, fmt.Errorf("ParseRequest: failed to json.Unmarshal: %w", err)
	}

	return response, nil
}
