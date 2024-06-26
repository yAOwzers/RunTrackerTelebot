package chatmanager

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"run-tracker-telebot/src/log"
	databasemanager "run-tracker-telebot/src/pkg/database-manager"
	imageprocessor "run-tracker-telebot/src/pkg/image-processor"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/conversation"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/message"
)

type Update struct {
	UpdateID int     `json:"update_id"`
	Message  Message `json:"message"`
}

type Message struct {
	Text string `json:"text"`
	Chat Chat   `json:"chat"`
}

type Chat struct {
	Id int `json:"id"`
}

type ChatManager struct {
	Bot             *gotgbot.Bot
	DatabaseManager *databasemanager.DatabaseManager
	ImageProcessor  *imageprocessor.ImageProcessor
	Token           string
	AuthorizedUsers map[int]bool
}

const TELEGRAM_FILE_URL = "https://api.telegram.org/file/bot"

func NewChatManager(databaseManager *databasemanager.DatabaseManager, imageProcessor *imageprocessor.ImageProcessor) *ChatManager {
	token, exists := os.LookupEnv("TELEGRAM_BOT_TOKEN")
	if !exists {
		log.Warn().Msgf("TELEGRAM_BOT_TOKEN not found in environment variables.")
	}

	log.Debug().Msgf("Token: %s", token)

	bot, err := gotgbot.NewBot(token, &gotgbot.BotOpts{
		BotClient: &gotgbot.BaseBotClient{
			Client:             http.Client{},
			UseTestEnvironment: false,
			DefaultRequestOpts: &gotgbot.RequestOpts{
				Timeout: gotgbot.DefaultTimeout,
				APIURL:  gotgbot.DefaultAPIURL,
			},
		},
	})

	if err != nil {
		panic("failed to create new bot: " + err.Error())
	}

	return &ChatManager{
		Bot:             bot,
		DatabaseManager: databaseManager,
		ImageProcessor:  imageProcessor,
		Token:           token,
	}
}

const (
	USER       = "user"
	DURATION   = "duration"
	WEEKRANGE  = "weekrange"
	MONTHRANGE = "monthrange"
	ONBOARD    = "onboard"
	HELP       = "help"
	AUTH       = "auth"
)

const HELP_MANUAL = "Commands:\n" +
	"/start - Start the bot\n" +
	"/historyUser - Get your workout history\n" +
	"/historyAll - Get all workout history for the group\n" +
	"/getdistance - Get total distance for a specified date range (month or week)\n" +
	"/delete - Delete a workout entry\n" +
	"/cancel - Cancel the current operation\n" +
	"/help - Show this help message\n" +
	"Send a workout image to log the details"

func (cm *ChatManager) Start() {

	// Create updater and dispatcher.
	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{
		// If an error is returned by a handler, log it and continue going.
		Error: func(b *gotgbot.Bot, ctx *ext.Context, err error) ext.DispatcherAction {
			log.Warn().Msgf("an error occurred while handling update:", err.Error())
			return ext.DispatcherActionNoop
		},
		MaxRoutines: ext.DefaultMaxRoutines,
	})

	log.Debug().Msgf("Loading authorized users...")
	cm.DatabaseManager.LoadUserData()

	updater := ext.NewUpdater(dispatcher, nil)

	// Add handlers for commands and messages
	// dispatcher.AddHandler(handlers.NewCommand("start", cm.handleStart))
	dispatcher.AddHandler(handlers.NewConversation(
		[]ext.Handler{handlers.NewCommand("start", cm.handleStart)},
		map[string][]ext.Handler{
			AUTH:    {handlers.NewMessage(noCommands, cm.handleAuth)},
			ONBOARD: {handlers.NewMessage(noCommands, cm.handleOnboard)},
			HELP:    {handlers.NewMessage(noCommands, cm.handleHelp)},
		},
		&handlers.ConversationOpts{
			Exits:        []ext.Handler{handlers.NewCommand("cancel", cm.handleCancel)},
			StateStorage: conversation.NewInMemoryStorage(conversation.KeyStrategySenderAndChat),
			AllowReEntry: true,
		},
	))

	dispatcher.AddHandler(handlers.NewCommand("historyUser", cm.middleWareAuth(cm.handleUserHistory)))
	dispatcher.AddHandler(handlers.NewCommand("historyAll", cm.middleWareAuth(cm.handleAllHistory)))
	dispatcher.AddHandler(handlers.NewConversation(
		[]ext.Handler{handlers.NewCommand("delete", cm.middleWareAuth(cm.handleWelcomeDelete))},
		map[string][]ext.Handler{
			USER: {handlers.NewMessage(noCommands, cm.handleDelete)},
		},
		&handlers.ConversationOpts{
			Exits:        []ext.Handler{handlers.NewCommand("cancel", cm.handleCancel)},
			StateStorage: conversation.NewInMemoryStorage(conversation.KeyStrategySenderAndChat),
			AllowReEntry: true,
		},
	))

	dispatcher.AddHandler(handlers.NewConversation(
		[]ext.Handler{handlers.NewCommand("getdistance", cm.middleWareAuth(cm.handleWelcomeDistance))},
		map[string][]ext.Handler{
			DURATION:   {handlers.NewMessage(noCommands, cm.handleDurationDecision)},
			WEEKRANGE:  {handlers.NewMessage(noCommands, cm.handleWeekRange)},
			MONTHRANGE: {handlers.NewMessage(noCommands, cm.handleMonthRange)},
		},
		&handlers.ConversationOpts{
			Exits:        []ext.Handler{handlers.NewCommand("cancel", cm.handleCancel)},
			StateStorage: conversation.NewInMemoryStorage(conversation.KeyStrategySenderAndChat),
			AllowReEntry: true,
		},
	))

	dispatcher.AddHandler(handlers.NewCommand("help", cm.handleHelp))
	dispatcher.AddHandler(handlers.NewMessage(message.Photo, cm.handleImage))

	err := updater.StartPolling(cm.Bot, &ext.PollingOpts{
		DropPendingUpdates: true,
		GetUpdatesOpts: &gotgbot.GetUpdatesOpts{
			Timeout: 9,
			RequestOpts: &gotgbot.RequestOpts{
				Timeout: time.Second * 10,
			},
		},
	})
	if err != nil {
		panic("failed to start polling: " + err.Error())
	}
	log.Printf("%s has been started...\n", cm.Bot.User.Username)

	// Idle, to keep updates coming in, and avoid bot stopping.
	updater.Idle()
}

func (cm *ChatManager) handleAuth(b *gotgbot.Bot, ctx *ext.Context) error {

	userInput := ctx.EffectiveMessage.Text

	if !isValidPassword(userInput) {
		log.Debug().Msgf("Invalid password: %s", userInput)
		_, err := ctx.EffectiveMessage.Reply(b, "Bro you sure you're authorized?", nil)
		if err != nil {
			log.Warn().Msgf("failed to send message:", err)
			return fmt.Errorf("failed to send message: %w", err)
		}
		return fmt.Errorf("invalid password")
	}

	log.Debug().Msgf("Password is valid: %s", userInput)
	log.Debug().Msgf("Passing to next state: %s", ONBOARD)

	_, err := ctx.EffectiveMessage.Reply(b, "Password is valid! Please share with me your name :)", nil)
	if err != nil {
		log.Warn().Msgf("failed to send message:", err)
		return fmt.Errorf("failed to send message: %w", err)
	}

	return handlers.NextConversationState(ONBOARD)
}

func isValidPassword(userInput string) bool {
	log.Debug().Msgf("Checking password: %s", userInput)

	secretPassword, exists := os.LookupEnv("SECRET_PASSWORD")
	if !exists {
		log.Warn().Msgf("SECRET_PASSWORD not found in environment variables, please set that up!")
		return false
	}

	return userInput == secretPassword
}

func noCommands(msg *gotgbot.Message) bool {
	return message.Text(msg) && !message.Command(msg)
}

func (cm *ChatManager) handleCancel(b *gotgbot.Bot, ctx *ext.Context) error {
	_, err := ctx.EffectiveMessage.Reply(b, "Oh, goodbye!", &gotgbot.SendMessageOpts{
		ParseMode: "html",
	})
	if err != nil {
		return fmt.Errorf("failed to send cancel message: %w", err)
	}
	return handlers.EndConversation()
}

func (cm *ChatManager) middleWareAuth(f func(*gotgbot.Bot, *ext.Context) error) func(*gotgbot.Bot, *ext.Context) error {

	return func(b *gotgbot.Bot, ctx *ext.Context) error {
		if !ctx.EffectiveUser.IsBot && ctx.EffectiveUser.Id != 0 && cm.DatabaseManager.IsAuthorizedUser(ctx.EffectiveUser.Id) {
			return f(b, ctx)
		}
		log.Warn().Msgf("Unauthorized user, or user is a bot, or user has invalid id: %d", ctx.EffectiveUser.Id)
		_, err := b.SendMessage(ctx.EffectiveChat.Id, "You are not authorized to use this bot, use /start to authenticate.", nil)
		if err != nil {
			log.Warn().Msgf("failed to send message:", err)
			return fmt.Errorf("failed to send message: %w", err)
		}
		return nil
	}
}

func (cm *ChatManager) handleStart(b *gotgbot.Bot, ctx *ext.Context) error {
	_, err := b.SendMessage(ctx.EffectiveChat.Id, "Hi! Before we start, what is the secret password?", nil)
	if err != nil {
		log.Warn().Msgf("failed to send message:", err)
		return fmt.Errorf("failed to send message: %w", err)

	}
	log.Debug().Msgf("Passing to next state: %s", AUTH)
	return handlers.NextConversationState(AUTH)
}

func (cm *ChatManager) handleOnboard(b *gotgbot.Bot, ctx *ext.Context) error {

	userInput := ctx.EffectiveMessage.Text
	log.Debug().Msgf("User Input: %s", userInput)

	// Save the user's name, only allow to save your own name
	log.Debug().Msgf("Saving user's name: %s", userInput)
	cm.DatabaseManager.SaveUser(userInput, ctx.EffectiveUser.Id)

	_, err := ctx.EffectiveMessage.Reply(b, "Welcome "+userInput+"! Send me a workout image and I will log the details.\n"+HELP_MANUAL, nil)
	if err != nil {
		log.Warn().Msgf("failed to send message:", err)
		return fmt.Errorf("failed to send message: %w", err)
	}

	return handlers.NextConversationState(HELP)
}

func (cm *ChatManager) handleHelp(b *gotgbot.Bot, ctx *ext.Context) error {
	helpMessage := "Welcome to Run Tracker Bot!\n" +
		HELP_MANUAL

	_, err := b.SendMessage(ctx.EffectiveChat.Id, helpMessage, nil)
	if err != nil {
		log.Warn().Msgf("failed to send help message:", err)
		return fmt.Errorf("failed to send help message: %w", err)
	}

	return err
}

func (cm *ChatManager) handleAllHistory(b *gotgbot.Bot, ctx *ext.Context) error {

	chatID := ctx.EffectiveChat.Id

	groupWorkouts, err := cm.DatabaseManager.GetAllWorkouts(chatID)
	if err != nil {
		log.Warn().Msgf("Workouts do not exist for groupID %d: %v", chatID, err)
		_, err := b.SendMessage(ctx.EffectiveChat.Id, "No Workout history exists, please submit images to begin!", nil)
		return err
	}

	// Prepare a string to send as a message
	var message string
	// message += fmt.Sprintf("Workouts for Group %d:\n", chatID)

	// Iterate over the groupWorkouts map
	for userId, dates := range groupWorkouts {
		username, err := cm.DatabaseManager.GetUsernameFromId(userId)
		if err != nil {
			log.Warn().Msgf("Error getting username for user %d: %v", userId, err)
			return err
		}

		message += fmt.Sprintf("User: %s\n", username)
		for date, workoutEntry := range dates {
			message += fmt.Sprintf("Date: %s\n", date)
			message += fmt.Sprintf("- Distance: %sKM, Pace: %s \n", workoutEntry.Distance, workoutEntry.Pace)
		}
	}

	// Process and send all workouts for the group
	log.Info().Msgf("All Workouts for Group %d: %v\n", chatID, groupWorkouts)

	_, err = ctx.EffectiveMessage.Reply(b, "Workouts for Group\n"+message+"\n\n", nil)
	if err != nil {
		log.Warn().Msgf("Error sending message to user in telegram:", err)
		return err
	}
	return nil
}

func (cm *ChatManager) handleUserHistory(b *gotgbot.Bot, ctx *ext.Context) error {

	chatID := ctx.EffectiveChat.Id
	userID := ctx.EffectiveUser.Id

	userWorkouts, err := cm.DatabaseManager.GetUserWorkouts(chatID, userID)
	if err != nil {
		log.Warn().Msgf("Error getting workouts for user %d in group %d: %v", userID, chatID, err)
		_, err := ctx.EffectiveMessage.Reply(b, "No existing workout history for user in group.", nil)
		if err != nil {
			log.Warn().Msgf("Error sending message to user in telegram:", err)
		}
		return err
	}

	var message string
	username, err := cm.DatabaseManager.GetUsernameFromId(userID)
	if err != nil {
		log.Warn().Msgf("Error getting username for user %d: %v", userID, err)
		return err
	}

	message += fmt.Sprintf("User: %s\n", username)
	for date, workout := range userWorkouts {
		message += fmt.Sprintf("Date: %s\n", date)
		message += fmt.Sprintf("- Distance: %sKM, Pace: %s \n\n", workout.Distance, workout.Pace)
	}

	// Process and send workouts for the specified user
	log.Info().Msgf("Workouts for User %d: %v\n", userID, userWorkouts)
	_, err = ctx.EffectiveMessage.Reply(b, "Workouts for User!\n "+message+"\n", nil)
	if err != nil {
		log.Warn().Msgf("Error sending message to user in telegram:", err)
		return err
	}

	log.Debug().Msgf("Releasing lock...")
	// workouts, ok := cm.DatabaseManager.Data.Workouts[ctx.EffectiveUser.Id]
	// if !ok {
	// 	log.Warn().Msgf("No workout history found.")
	// 	_, err := b.SendMessage(ctx.EffectiveChat.Id, "No workout history found.", nil)
	// 	return err
	// }

	// response := "Your workout history:\n"
	// for _, entry := range workouts {
	// 	entryTimestamp, err := strconv.ParseInt(entry.Timestamp, 10, 64)
	// 	if err != nil {
	// 		log.Warn().Msgf("Error parsing timestamp:", err)
	// 		return err
	// 	}

	// 	timestamp := time.Unix(entryTimestamp, 0)
	// 	response += timestamp.Format("2006-01-02 15:04:05") + ": " + entry.Text + "\n"
	// }

	// _, err := b.SendMessage(ctx.EffectiveChat.Id, response, nil)
	return nil
}

func isVerifiedDateFormat(date string) bool {

	// check if date is in the form of "2006-01-02"

	const layout = "2006-01-02"
	_, err := time.Parse(layout, date)
	if err != nil {
		log.Warn().Msgf("Date is not in the right form: ", err)
		return false
	}

	return true
}

func (cm *ChatManager) handleDelete(b *gotgbot.Bot, ctx *ext.Context) error {
	chatID := ctx.EffectiveChat.Id
	userID := ctx.EffectiveUser.Id

	// Wait for user's response
	// Add handler to capture the user's response
	dateInput := ctx.EffectiveMessage.Text

	if !isVerifiedDateFormat(dateInput) {
		log.Warn().Msgf("Invalid date format. Please provide the date in the format YYYY-MM-DD.")
		_, err := ctx.EffectiveMessage.Reply(b, "Invalid date format. Please provide the date in the format YYYY-MM-DD.", nil)
		if err != nil {
			log.Warn().Msgf("Error sending message to user in telegram:", err)
			return err
		}
		return fmt.Errorf("Invalid date format. Please provide the date in the format YYYY-MM-DD.")
	}

	if cm.DatabaseManager.DeleteWorkout(chatID, userID, dateInput) {
		_, err := ctx.EffectiveMessage.Reply(b, "Workout entry deleted successfully.", nil)
		if err != nil {
			log.Warn().Msgf("Error sending message to user in telegram:", err)
			return err
		}
	} else {
		_, err := ctx.EffectiveMessage.Reply(b, "No workout entry found for the provided date.", nil)
		if err != nil {
			log.Warn().Msgf("Error sending message to user in telegram:", err)
			return err
		}
	}

	return nil

}

func (cm *ChatManager) handleWelcomeDelete(b *gotgbot.Bot, ctx *ext.Context) error {
	// Prompt the user to provide the date of the workout entry to delete
	_, err := ctx.EffectiveMessage.Reply(b, "Please provide the date of the workout entry you want to delete (format: YYYY-MM-DD):", nil)
	if err != nil {
		log.Warn().Msgf("Error sending message to user in telegram:", err)
		return err
	}

	return handlers.NextConversationState(USER)
}

func (cm *ChatManager) handleWelcomeDistance(b *gotgbot.Bot, ctx *ext.Context) error {
	// Prompt the user to provide the date of the workout entry to delete
	_, err := b.SendMessage(ctx.EffectiveChat.Id, "Do you want to search by WEEK or MONTH?:", nil)
	if err != nil {
		log.Warn().Msgf("Error sending message to user in telegram:", err)
		return err
	}

	log.Debug().Msgf("Passing to next state: %s", DURATION)
	return handlers.NextConversationState(DURATION)
}

func (cm *ChatManager) handleDurationDecision(b *gotgbot.Bot, ctx *ext.Context) error {
	// Prompt the user to provide the date of the workout entry to delete
	userInput := ctx.EffectiveMessage.Text
	log.Debug().Msgf("User Input: %s", userInput)

	if userInput == "WEEK" {
		log.Debug().Msgf("Passing to next state: %s", WEEKRANGE)
		_, err := ctx.EffectiveMessage.Reply(b, "Please Enter the Date Range that you want to search (startDate, endDate) (format: YYYY-MM-DD, YYYY-MM-DD), example (2024-05-01, 2024-05-10):", nil)
		if err != nil {
			log.Warn().Msgf("Error sending message to user in telegram:", err)
			return err
		}

		log.Debug().Msgf("Passing to next state: %s", WEEKRANGE)
		return handlers.NextConversationState(WEEKRANGE)
	} else if userInput == "MONTH" {
		log.Debug().Msgf("Passing to next state: %s", MONTHRANGE)

		_, err := ctx.EffectiveMessage.Reply(b, "Which Month do you want to search? (format: YYYY-MM) (example: 2024-01)", nil)
		if err != nil {
			log.Warn().Msgf("Error sending message to user in telegram:", err)
			return err
		}

		return handlers.NextConversationState(MONTHRANGE)
	} else {
		log.Debug().Msgf("Invalid input: %s", userInput)
		_, err := ctx.EffectiveMessage.Reply(b, "You've just entered an invalid input, please use the words WEEK or MONTH", nil)
		if err != nil {
			log.Warn().Msgf("Error sending message to user in telegram:", err)
			return err
		}

		return err
	}

}

func (cm *ChatManager) handleWeekRange(b *gotgbot.Bot, ctx *ext.Context) error {

	userInput := ctx.EffectiveMessage.Text

	if !isValidDateRange(userInput) {
		log.Warn().Msgf("Invalid date range format. Please provide the date range in the format startDate, endDate.")
		_, err := ctx.EffectiveMessage.Reply(b, "Invalid date range format. Please provide the date range in the format startDate, endDate.", nil)
		if err != nil {
			log.Warn().Msgf("Error sending message to user in telegram:", err)
			return err
		}
		return fmt.Errorf("Invalid date range format. Please provide the date range in the format startDate, endDate.")
	}

	dateRange := strings.TrimSpace(userInput)
	log.Debug().Msgf("Splitting the dateRange with delimiter ',': %s", dateRange)
	// Split by comma to get startDate and endDate
	dates := strings.Split(dateRange, ",")
	if len(dates) != 2 {
		log.Warn().Msgf("Invalid date range format. Please provide the date range in the format startDate, endDate.")
	}

	startDate := strings.TrimSpace(dates[0])
	endDate := strings.TrimSpace(dates[1])

	// Process the date range
	totalDistanceByUser, err := cm.DatabaseManager.GetTotalDistanceByWeek(ctx.EffectiveChat.Id, startDate, endDate)
	if err != nil {
		log.Warn().Msgf("Error getting total distance for user: %v", err)
		_, err := b.SendMessage(ctx.EffectiveChat.Id, "Error getting total distance for user.", nil)
		if err != nil {
			log.Warn().Msgf("Error sending message to user in telegram:", err)
			return err
		}
		return err
	}

	var message string
	message += fmt.Sprintf("Total Distance for each user: \n")
	for userId, distance := range totalDistanceByUser {
		username, err := cm.DatabaseManager.GetUsernameFromId(userId)
		if err != nil {
			log.Warn().Msgf("Error getting username for user %d: %v", userId, err)
			return err
		}

		message += fmt.Sprintf("User: %s, Total Distance: %sKM\n", username, distance)
	}

	// Prompt the user to provide the date of the workout entry to delete
	_, err = ctx.EffectiveMessage.Reply(b, message, nil)
	if err != nil {
		log.Warn().Msgf("Error sending message to user in telegram:", err)
		return err
	}

	return nil
}

func isValidDateRange(dateRange string) bool {
	// Trim any surrounding whitespace
	dateRange = strings.TrimSpace(dateRange)

	log.Debug().Msgf("Splitting the dateRange with delimiter ',': %s", dateRange)
	// Split by comma to get startDate and endDate
	dates := strings.Split(dateRange, ",")
	if len(dates) != 2 {
		log.Warn().Msgf("Invalid date range format. Please provide the date range in the format startDate, endDate.")
		return false
	}

	startDateStr := strings.TrimSpace(dates[0])
	endDateStr := strings.TrimSpace(dates[1])

	// Define the date layout
	const layout = "2006-01-02"

	// Parse startDate
	startDate, err := time.Parse(layout, startDateStr)
	if err != nil {
		log.Warn().Msgf("Error parsing startDate:", err)
		return false
	}

	// Parse endDate
	endDate, err := time.Parse(layout, endDateStr)
	if err != nil {
		log.Warn().Msgf("Error parsing endDate:", err)
		return false
	}

	// Check if endDate is not earlier than startDate
	if endDate.Before(startDate) {
		log.Warn().Msgf("endDate is earlier than startDate")
		return false
	}

	// If all checks pass
	return true
}

func (cm *ChatManager) handleMonthRange(b *gotgbot.Bot, ctx *ext.Context) error {
	userInput := ctx.EffectiveMessage.Text

	if !isValidMonthAndYear(userInput) {
		log.Warn().Msgf("Invalid month format. Please provide the month in the format YYYY-MM.")
		_, err := ctx.EffectiveMessage.Reply(b, "Invalid month format. Please provide the month in the format YYYY-MM.", nil)
		if err != nil {
			log.Warn().Msgf("Error sending message to user in telegram:", err)
			return err
		}
		return err
	}

	userInput = strings.TrimSpace(userInput)

	log.Debug().Msgf("Splitting the dateRange with delimiter '-': %s", userInput)
	// Split by comma to get startDate and endDate
	dates := strings.Split(userInput, "-")
	if len(dates) != 2 {
		log.Warn().Msgf("Invalid date range format. Please provide the date range in the format startDate, endDate.")
	}

	year := strings.TrimSpace(dates[0])
	month := strings.TrimSpace(dates[1])

	totalDistanceByUser, err := cm.DatabaseManager.GetTotalDistanceByMonth(ctx.EffectiveChat.Id, month, year)
	if err != nil {
		log.Warn().Msgf("Error getting total distance for user: %v", err)
		_, err := b.SendMessage(ctx.EffectiveChat.Id, "Error getting total distance for user.", nil)
		if err != nil {
			log.Warn().Msgf("Error sending message to user in telegram:", err)
			return err
		}
		return err
	}

	convertedMonth, err := convertNumToMonth(month)
	if err != nil {
		log.Warn().Msgf("Error converting month to string: %v", err)
		return err
	}

	var message string
	message += fmt.Sprintf("Total Distance for each user in " + convertedMonth + " : \n")
	for userId, distance := range totalDistanceByUser {
		username, err := cm.DatabaseManager.GetUsernameFromId(userId)
		if err != nil {
			log.Warn().Msgf("Error getting username for user %d: %v", userId, err)
			return err
		}

		message += fmt.Sprintf("User: %s, Total Distance: %sKM\n", username, distance)
	}

	_, err = ctx.EffectiveMessage.Reply(b, message, nil)
	if err != nil {
		log.Warn().Msgf("Error sending message to user in telegram:", err)
		return err
	}

	return nil
}

func convertNumToMonth(month string) (string, error) {
	months := map[string]string{
		"01": "JAN",
		"02": "FEB",
		"03": "MAR",
		"04": "APR",
		"05": "MAY",
		"06": "JUN",
		"07": "JUL",
		"08": "AUG",
		"09": "SEP",
		"10": "OCT",
		"11": "NOV",
		"12": "DEC",
	}

	if monthName, exists := months[month]; exists {
		return monthName, nil
	}

	return "", errors.New("invalid month format")
}

func isValidMonthAndYear(userInput string) bool {
	// Normalize input to uppercase for case-insensitive comparison
	userInput = strings.ToUpper(strings.TrimSpace(userInput))

	// List of valid month abbreviations
	const layout = "2006-01"

	// Parse startDate
	_, err := time.Parse(layout, userInput)
	if err != nil {
		log.Warn().Msgf("Error parsing startDate:", err)
		return false
	}

	return true
}

func (cm *ChatManager) downloadFile(url string, filepath string, ctx *ext.Context) error {
	// Download the file
	log.Debug().Msgf("Downloading image file from %s", url)
	resp, err := http.Get(url)
	if err != nil {
		log.Warn().Msgf("Error downloading image file:", err)
		_, err := cm.Bot.SendMessage(ctx.EffectiveChat.Id, "Error processing image. Please try again.", nil)
		return err
	}

	if resp.StatusCode != http.StatusOK {
		log.Warn().Msgf("Bad Status Code:", resp.StatusCode)
		log.Warn().Msgf("Error downloading image file:", err)
		_, err := cm.Bot.SendMessage(ctx.EffectiveChat.Id, "Error processing image. Please try again.", nil)
		return err
	}

	defer resp.Body.Close()

	// Save the image to a file
	imagePath := "image.jpg"
	out, err := os.Create(imagePath)
	if err != nil {
		log.Warn().Msgf("Error saving image file:", err)
		_, err := cm.Bot.SendMessage(ctx.EffectiveChat.Id, "Error processing image. Please try again.", nil)
		return err
	}

	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		log.Warn().Msgf("Error saving image file:", err)
		_, err := cm.Bot.SendMessage(ctx.EffectiveChat.Id, "Error processing image. Please try again.", nil)
		return err
	}

	log.Info().Msgf("Image saved to %s", imagePath)
	return nil
}

func (cm *ChatManager) handleImage(b *gotgbot.Bot, ctx *ext.Context) error {

	userId := ctx.EffectiveUser.Id
	chatId := ctx.EffectiveChat.Id

	log.Debug().Msgf("Handling image...")
	if ctx.Message.Photo == nil {
		_, err := ctx.EffectiveMessage.Reply(b, "Please send a valid image.", nil)
		if err != nil {
			log.Warn().Msgf("Error sending message to user in telegram:", err)
			return err
		}
		log.Warn().Msgf("No image found in message.")
		return err
	}

	photo := ctx.Message.Photo[len(ctx.Message.Photo)-1]

	/*
		Use this method to get basic information about a file and prepare it for downloading.
		For the moment, bots can download files of up to 20MB in size.
		On success, a File object is returned.
		The file can then be downloaded via the link https://api.telegram.org/file/bot<token>/<file_path>,
		where <file_path> is taken from the response.
	*/

	log.Debug().Msgf("Getting file...")
	file, err := b.GetFile(photo.FileId, nil)
	if err != nil {
		log.Warn().Msgf("Error getting image file:", err)
		_, err := b.SendMessage(ctx.EffectiveChat.Id, "Error processing image. Please try again.", nil)
		return err
	}

	log.Debug().Msgf("Received file: %v", file)

	imagePath := "image.jpg"
	log.Info().Msgf("Downloading image file into %s", imagePath)

	log.Debug().Msgf("File Download Path: %s", file.FilePath)
	cm.downloadFile(TELEGRAM_FILE_URL+cm.Token+"/"+file.FilePath, imagePath, ctx)

	text, err := cm.ImageProcessor.ProcessImage(imagePath)
	if err != nil {
		log.Warn().Msgf("Error processing image:", err)
		_, err := b.SendMessage(ctx.EffectiveChat.Id, "Error processing image. Please try again.", nil)
		return err
	}

	isApple := cm.ImageProcessor.IsAppleWorkout(text)
	var workoutDetails map[string]string
	if isApple {
		workoutDetails, err = cm.ImageProcessor.ParseWorkoutDetails(text)
		if err != nil {
			log.Warn().Msgf("Error extracting workout details:", err)
			_, err := b.SendMessage(ctx.EffectiveChat.Id, "Error extracting workout details. Please try again.", nil)
			return err
		}
		log.Debug().Msgf("Workout details: %v", workoutDetails)
	} else if cm.ImageProcessor.IsRunKeeper(text) {
		workoutDetails, err = cm.ImageProcessor.ParseRunKeepWorkoutDetails(text)
		if err != nil {
			log.Warn().Msgf("Error extracting workout details:", err)
			_, err := b.SendMessage(ctx.EffectiveChat.Id, "Error extracting workout details. Please try again.", nil)
			return err
		}
		log.Debug().Msgf("Workout details: %v", workoutDetails)
	}

	// Save the workout data
	log.Debug().Msgf("Locking the database")
	cm.DatabaseManager.Data.Lock()

	date := time.Now().Format("2006-01-02")
	didInsert := cm.DatabaseManager.InsertWorkoutEntry(chatId, userId, date, workoutDetails)

	// cm.DatabaseManager.Data.Workouts[ctx.EffectiveUser.Id] = append(cm.DatabaseManager.Data.Workouts[ctx.EffectiveUser.Id], databasemanager.WorkoutEntry{
	// 	Text:      strings.Join([]string{workoutDetails["Distance"], workoutDetails["Pace"], workoutDetails["HeartRate"]}, ", "),
	// 	Timestamp: timestamp,
	// })

	log.Debug().Msgf("Unlocking the database")
	cm.DatabaseManager.Data.Unlock()

	if didInsert {
		err = cm.DatabaseManager.SaveData()
		if err != nil {
			log.Warn().Msgf("Error saving workout data:", err)
			_, err := b.SendMessage(ctx.EffectiveChat.Id, "Error saving workout data.", nil)
			return err
		}

		_, err = ctx.EffectiveMessage.Reply(b, "Workout logged!\n"+
			"Date: "+workoutDetails["Date"]+"\n"+
			"Distance: "+workoutDetails["Distance"]+"KM\n"+
			"Avg Pace: "+workoutDetails["Pace"]+"\n", nil)
		return err
	} else {
		_, err = ctx.EffectiveMessage.Reply(b, "Invalid workout details. No insertion performed into database.", nil)
		if err != nil {
			log.Warn().Msgf("Error sending message to user in telegram:", err)
			return err
		}
		log.Warn().Msgf("Invalid workout details. No insertion performed into database.")
		return err
	}
}
