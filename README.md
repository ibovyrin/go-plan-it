# Go Plan IT! 📅

## About
Go Plan IT is a Telegram bot that helps you to manage your Google Calendar events. With this bot, you can create, retrieve, and receive notifications about your Google Calendar events directly in Telegram.

**Note**: All user data, including calendar ids, and chat ids are stored locally.

## Features
- **Event Retrieval**: Fetches upcoming events from the user’s Google Calendar and displays them in the Telegram chat.
- **Event Creation**: Allows the user to create new events via Telegram, parses it with ChatGPT and automatically adds them to the Google Calendar.
- **Daily Agenda**: Sends a daily agenda to the user with all of the day's events.
- **Event Notifications**: Notifies the user about upcoming events.

## Setup and Run

### Prerequisites
Ensure you have Go installed, and have set up a bot on Telegram to obtain the API Token. You'll also need credentials from the Google API Console for OAuth 2.0.

### Installation
1. Clone this repository:
   ```sh
   git clone git@github.com:ibovyrin/go-plan-it.git
   ```
2. Install dependencies:
   ```sh
   go get
   ```
3. Replace the placeholders in with actual values:
   ```sh
   export OPENAI_TOKEN="OPENAI_TOKEN"  
   export TG_BOT_ALLOW_LIST="user1,user2"  
   export TG_BOT_TOKEN="TG_BOT_TOKEN"  
   export WEBHOOK_URL="WEBHOOK_URL/webhook"
   ```
4. Build:
   ```sh
   go build ./cmd/go-plan-it
   ```
5. Run:
   ```sh
   ./go-plan-it
   ```
### Usage
- `/start`: Begin using the bot and authenticate with Google.
- `/stop`: Stop using the bot and delete stored data.
- `/events`: Display upcoming events.
- `/new`: Create a new event.
- `/watch`: Subscribe to a Google Calendar.
- `/stopwatch`: Remove a Google Calendar subscription.

## License
This project is licensed under the Apache License. See the [LICENSE.md](LICENSE.md) file for details.

## Disclaimer
This code is for educational and demonstration purposes only. Always review and understand the code fully before using in a production environment.