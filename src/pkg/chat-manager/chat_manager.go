package chatmanager

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"run-tracker-telebot/src/log"
	databasemanager "run-tracker-telebot/src/pkg/database-manager"
	imageprocessor "run-tracker-telebot/src/pkg/image-processor"
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
			Client: http.Client{},
			DefaultRequestOpts: &gotgbot.RequestOpts{
				Timeout: gotgbot.DefaultTimeout, // Customise the default request timeout here
				APIURL:  gotgbot.DefaultAPIURL,  // As well as the Default API URL here (in case of using local bot API servers)
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
	USER = "user"
	WEEK = "week"
)

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

	updater := ext.NewUpdater(dispatcher, nil)

	// Add handlers for commands and messages
	dispatcher.AddHandler(handlers.NewCommand("start", cm.handleStart))
	dispatcher.AddHandler(handlers.NewCommand("historyUser", cm.handleUserHistory))
	dispatcher.AddHandler(handlers.NewCommand("historyAll", cm.handleAllHistory))
	dispatcher.AddHandler(handlers.NewConversation(
		[]ext.Handler{handlers.NewCommand("delete", cm.handleWelcomeDelete)},
		map[string][]ext.Handler{
			USER: {handlers.NewMessage(noCommands, cm.handleDelete)},
		},
		&handlers.ConversationOpts{
			Exits:        []ext.Handler{handlers.NewCommand("cancel", cm.handleCancel)},
			StateStorage: conversation.NewInMemoryStorage(conversation.KeyStrategySenderAndChat),
			AllowReEntry: true,
		},
	))

	// dispatcher.AddHandler(handlers.NewConversation(
	// 	[]ext.Handler{handlers.NewCommand("getdistance", cm.handleWelcomeDistance)},
	// 	map[string][]ext.Handler{
	// 		WEEK: {handlers.NewMessage(noCommands, cm.handleDistance)},
	// 	},
	// 	&handlers.ConversationOpts{
	// 		Exits:        []ext.Handler{handlers.NewCommand("cancel", cm.handleCancel)},
	// 		StateStorage: conversation.NewInMemoryStorage(conversation.KeyStrategySenderAndChat),
	// 		AllowReEntry: true,
	// 	},
	// ))

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

func (cm *ChatManager) handleStart(b *gotgbot.Bot, ctx *ext.Context) error {
	_, err := b.SendMessage(ctx.EffectiveChat.Id, "Hi! Send me a workout image and I will log the details.", nil)
	return err
}

func (cm *ChatManager) handleHelp(b *gotgbot.Bot, ctx *ext.Context) error {
	helpMessage := "Welcome to Run Tracker Bot!\n" +
		"Commands:\n" +
		"/start - Start the bot\n" +
		"/historyUser - Get your workout history\n" +
		"/historyAll - Get all workout history for the group\n" +
		"/delete - Delete a workout entry\n" +
		"/cancel - Cancel the current operation\n" +
		"/help - Show this help message\n"

	_, err := ctx.EffectiveMessage.Reply(b, helpMessage, nil)
	if err != nil {
		log.Warn().Msgf("failed to send help message:", err)
		return fmt.Errorf("failed to send help message: %w", err)
	}
	return nil
}

func (cm *ChatManager) handleAllHistory(b *gotgbot.Bot, ctx *ext.Context) error {

	chatID := ctx.EffectiveChat.Id

	groupWorkouts, err := cm.DatabaseManager.GetAllWorkouts(chatID)
	if err != nil {
		log.Warn().Msgf("Error getting all workouts for group %d: %v", chatID, err)
		_, err := b.SendMessage(ctx.EffectiveChat.Id, "Error getting all workouts for group.", nil)
		return err
	}

	// Prepare a string to send as a message
	var message string
	// message += fmt.Sprintf("Workouts for Group %d:\n", chatID)

	// Iterate over the groupWorkouts map
	for userId, dates := range groupWorkouts {
		message += fmt.Sprintf("User ID: %d\n", userId)
		for date, workoutEntry := range dates {
			message += fmt.Sprintf("Date: %s\n", date)
			message += fmt.Sprintf("- Distance: %s, Pace: %s \n", workoutEntry.Distance, workoutEntry.Pace)
		}
	}

	// Process and send all workouts for the group
	log.Info().Msgf("All Workouts for Group %d: %v\n", chatID, groupWorkouts)

	_, err = ctx.EffectiveMessage.Reply(b, "Workouts for Group: "+message+"\n", nil)
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
	message += fmt.Sprintf("User ID: %d\n", userID)
	for date, workout := range userWorkouts {
		message += fmt.Sprintf("Date: %s\n", date)
		message += fmt.Sprintf("- Distance: %s, Pace: %s \n", workout.Distance, workout.Pace)
	}

	// Process and send workouts for the specified user
	log.Info().Msgf("Workouts for User %d: %v\n", userID, userWorkouts)
	_, err = ctx.EffectiveMessage.Reply(b, "Workouts for User:\n "+message+"\n", nil)
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

	workoutDetails, err := cm.ImageProcessor.ParseWorkoutDetails(text)
	if err != nil {
		log.Warn().Msgf("Error extracting workout details:", err)
		_, err := b.SendMessage(ctx.EffectiveChat.Id, "Error extracting workout details. Please try again.", nil)
		return err
	}

	log.Debug().Msgf("Workout details: %v", workoutDetails)

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
			"Distance: "+workoutDetails["Distance"]+"\n"+
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
