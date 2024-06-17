package chatmanager

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"run-tracker-telebot/src/log"
	databasemanager "run-tracker-telebot/src/pkg/database-manager"
	imageprocessor "run-tracker-telebot/src/pkg/image-processor"
	"strconv"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
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
	}
}

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
	dispatcher.AddHandler(handlers.NewCommand("history", cm.handleHistory))
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

func (cm *ChatManager) handleStart(b *gotgbot.Bot, ctx *ext.Context) error {
	_, err := b.SendMessage(ctx.EffectiveChat.Id, "Hi! Send me a workout image and I will log the details.")
	return err
}

// echo replies to a messages with its own contents.
func (cm *ChatManager) echo(b *gotgbot.Bot, ctx *ext.Context) error {
	_, err := ctx.EffectiveMessage.Reply(b, ctx.EffectiveMessage.Text, nil)
	if err != nil {
		log.Warn().Msgf("failed to echo message:", err)
		return fmt.Errorf("failed to echo message: %w", err)
	}
	return nil
}

func (cm *ChatManager) handleHistory(b *gotgbot.Bot, ctx *ext.Context) error {
	workouts, ok := cm.DatabaseManager.Data.Workouts[ctx.EffectiveUser.Id]
	if !ok {
		log.Warn().Msgf("No workout history found.")
		_, err := b.SendMessage(ctx.EffectiveChat.Id, "No workout history found.", nil)
		return err
	}

	response := "Your workout history:\n"
	for _, entry := range workouts {
		entryTimestamp, err := strconv.ParseInt(entry.Timestamp, 10, 64)
		if err != nil {
			log.Warn().Msgf("Error parsing timestamp:", err)
			return err
		}

		timestamp := time.Unix(entryTimestamp, 0)
		response += timestamp.Format("2006-01-02 15:04:05") + ": " + entry.Text + "\n"
	}

	_, err := b.SendMessage(ctx.EffectiveChat.Id, response, nil)
	return err
}

func (cm *ChatManager) downloadFile(url string, filepath string, ctx *ext.Context) error {
	// Download the file
	resp, err := http.Get(TELEGRAM_FILE_URL + cm.Token + "/" + filepath)
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
	if ctx.Message.Photo == nil {
		_, err := b.SendMessage(ctx.EffectiveChat.Id, "Please send a valid image.", nil)
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

	file, err := b.GetFile(photo.FileId, nil)
	if err != nil {
		log.Warn().Msgf("Error getting image file:", err)
		_, err := b.SendMessage(ctx.EffectiveChat.Id, "Error processing image. Please try again.", nil)
		return err
	}

	imagePath := "image.jpg"
	cm.downloadFile(TELEGRAM_FILE_URL+cm.Token+"/"+file.FilePath, imagePath, ctx)

	text, err := cm.ImageProcessor.ProcessImage(imagePath)
	if err != nil {
		log.Warn().Msgf("Error processing image:", err)
		_, err := b.SendMessage(ctx.EffectiveChat.Id, "Error processing image. Please try again.")
		return err
	}

	workoutDetails, err := cm.ImageProcessor.ParseWorkoutDetails(text)
	if err != nil {
		log.Warn().Msgf("Error extracting workout details:", err)
		_, err := b.SendMessage(ctx.EffectiveChat.Id, "Error extracting workout details. Please try again.")
		return err
	}

	// Save the workout data
	log.Debug().Msgf("Locking the database")
	cm.DatabaseManager.Data.Lock()

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	cm.DatabaseManager.Data.Workouts[ctx.EffectiveUser.Id] = append(cm.DatabaseManager.Data.Workouts[ctx.EffectiveUser.Id], databasemanager.WorkoutEntry{
		Text:      strings.Join([]string{workoutDetails["Distance"], workoutDetails["Pace"], workoutDetails["HeartRate"]}, ", "),
		Timestamp: timestamp,
	})
	cm.DatabaseManager.Data.Unlock()

	err = cm.DatabaseManager.SaveData()
	if err != nil {
		log.Warn().Msgf("Error saving workout data:", err)
		_, err := b.SendMessage(ctx.EffectiveChat.Id, "Error saving workout data.")
		return err
	}

	_, err = b.SendMessage(ctx.EffectiveChat.Id, "Workout logged!\n"+
		"Distance: "+workoutDetails["Distance"]+"\n"+
		"Avg Pace: "+workoutDetails["Pace"]+"\n"+
		"Avg Heart Rate: "+workoutDetails["HeartRate"])
	return err
}
