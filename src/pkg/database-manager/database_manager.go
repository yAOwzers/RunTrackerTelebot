package databasemanager

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"run-tracker-telebot/src/log"
	"sync"
)

type WorkoutData struct {
	Workouts map[int64][]WorkoutEntry `json:"workouts"`
	sync.Mutex
}

type WorkoutEntry struct {
	Text      string `json:"text"`
	Timestamp string `json:"timestamp"`
}

type DatabaseManager struct {
	FilePath string
	Data     *WorkoutData
}

const workoutDataDir = "data"
const workoutDataFile = "workout_data.json"

func NewDatabaseManager(filePath string) *DatabaseManager {
	return &DatabaseManager{
		FilePath: filePath,
		Data:     &WorkoutData{},
	}
}

func (db *DatabaseManager) LoadData() error {

	if _, err := os.Stat(workoutDataDir); os.IsNotExist(err) {
		log.Debug().Msgf("Data dir does not exist, Creating workout data directory")
		os.Mkdir(workoutDataDir, os.ModePerm)
	}

	if _, err := os.Stat(workoutDataDir + "/" + workoutDataFile); os.IsNotExist(err) {
		log.Debug().Msgf("Workout data file does not exist, Creating workout data file")
		file, err := os.Create(workoutDataDir + "/" + workoutDataFile)
		if err != nil {
			log.Warn().Msgf("Error creating workout data file: %v", err)
		}
		defer file.Close()
	}

	fileContent, err := ioutil.ReadFile(db.FilePath)
	if err != nil {
		log.Warn().Msgf("Error reading file: %v", err)
		return fmt.Errorf("error reading file: %v", err)
	}

	// Initialize dm.Data.Workouts map if nil
	if db.Data.Workouts == nil {
		log.Debug().Msgf("Initializing Workouts map")
		db.Data.Workouts = make(map[int64][]WorkoutEntry)
	}

	// Unmarshal JSON into WorkoutData struct
	if err := json.Unmarshal(fileContent, &db.Data); err != nil {
		log.Warn().Msgf("Error unmarshalling JSON: %v", err)
		return fmt.Errorf("error unmarshalling JSON: %v", err)
	}

	return nil
}

func (db *DatabaseManager) SaveData() error {
	db.Data.Lock()
	defer db.Data.Unlock()

	data, err := json.MarshalIndent(db.Data, "", "    ")
	if err != nil {
		log.Warn().Msgf("Error marshalling data: %v", err)
		return err
	}

	err = ioutil.WriteFile(db.FilePath, data, 0644)
	if err != nil {
		log.Warn().Msgf("Error writing to file: %v", err)
		return err
	}

	return nil
}
