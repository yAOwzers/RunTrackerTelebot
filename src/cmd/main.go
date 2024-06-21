package main

import (
	"os"
	"os/signal"
	"path/filepath"
	"run-tracker-telebot/src/log"
	chatmanager "run-tracker-telebot/src/pkg/chat-manager"
	databasemanager "run-tracker-telebot/src/pkg/database-manager"
	imageprocessor "run-tracker-telebot/src/pkg/image-processor"
	"run-tracker-telebot/src/pkg/shared"
	"syscall"

	"github.com/joho/godotenv"
)

const logsDir = "./logs"

func main() {
	log.InitLogger()
	log.Info().Msgf("Initiatized Logger")

	rootDir, err := os.Getwd()
	if err != nil {
		log.Warn().Msgf("Error getting current directory:", err)
	}
	err = godotenv.Load(filepath.Join(rootDir, ".env"))
	if err != nil {
		log.Warn().Msgf("Error loading .env file:", err)
	}

	imageProcessor := imageprocessor.NewImageProcessor()
	databaseManager := databasemanager.NewDatabaseManager(
		shared.WORKOUT_DATA_DIR+"/"+shared.WORKOUT_DATA_FILE,
		shared.WORKOUT_DATA_DIR+"/"+shared.AUTHORIZED_USERS_FILE,
	)
	chatManager := chatmanager.NewChatManager(databaseManager, imageProcessor)

	err = databaseManager.LoadData()
	if err != nil {
		log.Warn().Msgf("Error loading workout data: %v", err)
	}

	// Start the chat manager
	chatManager.Start()

	// Create a channel to receive OS signals
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	// Block until a signal is received
	<-sig

	log.Info().Msgf("Exiting...")
}
