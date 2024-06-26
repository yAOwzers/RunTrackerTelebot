package databasemanager

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"run-tracker-telebot/src/log"
	"run-tracker-telebot/src/pkg/shared"
	"strconv"
	"sync"
)

type WorkoutData struct {
	Workouts map[int64]map[int64]map[string]WorkoutEntry `json:"workouts"`
	sync.Mutex
}

type WorkoutEntry struct {
	Distance string `json:"distance"`
	Pace     string `json:"pace"`
}

type DatabaseManager struct {
	FilePath     string
	UserFilePath string
	Data         *WorkoutData
	UserData     *UserToIdMap
}

type UserToIdMap struct {
	Users map[int64]string `json:"users"`
	sync.Mutex
}

func NewDatabaseManager(filePath string, userFilepath string) *DatabaseManager {
	return &DatabaseManager{
		FilePath:     filePath,
		UserFilePath: userFilepath,
		Data:         &WorkoutData{},
	}
}

func NewWorkoutData() *WorkoutData {
	return &WorkoutData{
		Workouts: make(map[int64]map[int64]map[string]WorkoutEntry),
	}
}

func (db *DatabaseManager) LoadUserData() error {
	if _, err := os.Stat(shared.WORKOUT_DATA_DIR); os.IsNotExist(err) {
		log.Debug().Msgf("Data dir does not exist, Creating workout data directory: %v", shared.WORKOUT_DATA_DIR)
		os.Mkdir(shared.WORKOUT_DATA_DIR, os.ModePerm)
	}

	log.Debug().Msgf("Data Directory Exists...")

	if _, err := os.Stat(shared.WORKOUT_DATA_DIR + "/" + shared.AUTHORIZED_USERS_FILE); os.IsNotExist(err) {
		log.Debug().Msgf("User data file does not exist, Creating User data file: %v", shared.AUTHORIZED_USERS_FILE)
		file, err := os.Create(shared.WORKOUT_DATA_DIR + "/" + shared.AUTHORIZED_USERS_FILE)
		if err != nil {
			log.Warn().Msgf("Error creating User data file: %v", err)
		}
		defer file.Close()
	}

	fileContent, err := ioutil.ReadFile(shared.WORKOUT_DATA_DIR + "/" + shared.AUTHORIZED_USERS_FILE)
	if err != nil {
		log.Warn().Msgf("Error reading file: %v", err)
		return fmt.Errorf("error reading file: %v", err)
	}

	// Initialize dm.Data.Workouts map if nil
	if db.UserData == nil {
		log.Debug().Msgf("UserData empty, Initializing UserData map")
		db.UserData = &UserToIdMap{}
	}

	// Unmarshal JSON into WorkoutData struct
	if err := json.Unmarshal(fileContent, &db.UserData); err != nil {
		log.Warn().Msgf("Error unmarshalling JSON: %v", err)
		return fmt.Errorf("error unmarshalling JSON: %v", err)
	}

	return nil
}

func (db *DatabaseManager) SaveUserData() error {
	file, err := os.Create(db.UserFilePath)
	if err != nil {
		log.Warn().Msgf("Error creating file: %v", err)
		return err
	}
	defer file.Close()

	log.Debug().Msgf("Saving data to file: %v", db.UserFilePath)
	encoder := json.NewEncoder(file)

	log.Debug().Msgf("Encoding data: %v", db.UserData)
	err = encoder.Encode(db.UserData)
	if err != nil {
		log.Warn().Msgf("Error encoding data: %v", err)
		return err
	}

	return nil
}

func (db *DatabaseManager) AddWorkout(groupID, userID int64, date string, entry WorkoutEntry) {

	log.Debug().Msgf("Acquiring lock...")
	db.Data.Lock()
	defer db.Data.Unlock()

	if db.Data.Workouts[groupID] == nil {
		db.Data.Workouts[groupID] = make(map[int64]map[string]WorkoutEntry)
	}

	if db.Data.Workouts[groupID][userID] == nil {
		db.Data.Workouts[groupID][userID][date] = WorkoutEntry{}
	}

	db.Data.Workouts[groupID][userID][date] = entry

	log.Debug().Msgf("Added workout entry: %v", entry)
	db.SaveData()
}

func (db *DatabaseManager) GetUserWorkouts(groupID, userID int64) (map[string]WorkoutEntry, error) {
	log.Debug().Msgf("Acquiring lock...")
	db.Data.Lock()
	defer db.Data.Unlock()

	if db.Data.Workouts[groupID] == nil {
		log.Warn().Msgf("No workouts found for group: %v", groupID)
		return nil, fmt.Errorf("no workouts found for group: %v", groupID)
	}

	return db.Data.Workouts[groupID][userID], nil
}

func (db *DatabaseManager) GetAllWorkouts(groupID int64) (map[int64]map[string]WorkoutEntry, error) {
	log.Debug().Msgf("Acquiring lock...")
	db.Data.Lock()
	defer db.Data.Unlock()

	if db.Data.Workouts[groupID] == nil {
		log.Warn().Msgf("No workouts found for group: %v", groupID)
		return nil, fmt.Errorf("no workouts found for group: %v", groupID)
	}

	return db.Data.Workouts[groupID], nil
}

func (db *DatabaseManager) LoadData() error {

	if _, err := os.Stat(shared.WORKOUT_DATA_DIR); os.IsNotExist(err) {
		log.Debug().Msgf("Data dir does not exist, Creating workout data directory")
		os.Mkdir(shared.WORKOUT_DATA_DIR, os.ModePerm)
	}

	if _, err := os.Stat(shared.WORKOUT_DATA_DIR + "/" + shared.WORKOUT_DATA_FILE); os.IsNotExist(err) {
		log.Debug().Msgf("Workout data file does not exist, Creating workout data file")
		file, err := os.Create(shared.WORKOUT_DATA_DIR + "/" + shared.WORKOUT_DATA_FILE)
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
		db.Data.Workouts = make(map[int64]map[int64]map[string]WorkoutEntry)
	}

	// Unmarshal JSON into WorkoutData struct
	if err := json.Unmarshal(fileContent, &db.Data); err != nil {
		log.Warn().Msgf("Error unmarshalling JSON: %v", err)
		return fmt.Errorf("error unmarshalling JSON: %v", err)
	}

	return nil
}

func (db *DatabaseManager) SaveData() error {
	file, err := os.Create(db.FilePath)
	if err != nil {
		log.Warn().Msgf("Error creating file: %v", err)
		return err
	}
	defer file.Close()

	log.Debug().Msgf("Saving data to file: %v", db.FilePath)
	encoder := json.NewEncoder(file)

	log.Debug().Msgf("Encoding data: %v", db.Data)
	err = encoder.Encode(db.Data)
	if err != nil {
		log.Warn().Msgf("Error encoding data: %v", err)
		return err
	}

	return nil
}

func (db *DatabaseManager) InsertWorkoutEntry(chatID int64, userID int64, date string, workoutDetails map[string]string) bool {

	log.Debug().Msgf("Inserting Workout Entry in database: %v", workoutDetails)

	if workoutDetails["Distance"] == "" || workoutDetails["Pace"] == "" {
		log.Warn().Msgf("Invalid workout details: %v", workoutDetails)
		log.Warn().Msgf("No insertion performed into database")
		return false
	}

	if db.Data.Workouts == nil {
		log.Debug().Msgf("Initializing Workouts map")
		db.Data.Workouts = make(map[int64]map[int64]map[string]WorkoutEntry)
	}

	if db.Data.Workouts[chatID] == nil {
		log.Debug().Msgf("Initializing chatID map")
		db.Data.Workouts[chatID] = make(map[int64]map[string]WorkoutEntry)
	}

	if db.Data.Workouts[chatID][userID] == nil {
		log.Debug().Msgf("Initializing userID map")
		db.Data.Workouts[chatID][userID] = make(map[string]WorkoutEntry)
	}

	log.Debug().Msgf("Appending into Workout Database: %v", workoutDetails)
	db.Data.Workouts[chatID][userID][date] = WorkoutEntry{
		Distance: workoutDetails["Distance"],
		Pace:     workoutDetails["Pace"],
	}

	log.Info().Msgf("Workout entry inserted into database successfully: %v", workoutDetails)
	return true
}

func (db *DatabaseManager) DeleteWorkout(chatID int64, userID int64, date string) bool {
	log.Debug().Msgf("Acquiring lock...")
	db.Data.Lock()
	defer db.Data.Unlock()

	if db.Data.Workouts[chatID] == nil {
		log.Warn().Msgf("No workouts found for chat: %v", chatID)
		return false
	}
	// Check if the map at chatID level exists
	log.Debug().Msgf("Checking if chatID exists: %v", chatID)
	if userMap, ok := db.Data.Workouts[chatID]; ok {
		// Check if the map at userID level exists

		log.Debug().Msgf("Checking if userID exists: %v", userID)
		if dateMap, ok := userMap[userID]; ok {
			// Delete the date key

			log.Info().Msgf("Deleting workout entry for user: %v, date: %v", userID, date)
			delete(dateMap, date)

			// If the dateMap becomes empty after deletion, clean up the map
			log.Debug().Msgf("Checking if dateMap is empty after deletion: %v", len(dateMap))
			if len(dateMap) == 0 {
				log.Info().Msgf("DateMap is empty, deleting userMap: %v", userID)
				delete(userMap, userID)
			}

			log.Debug().Msgf("Checking if userMap is empty after deletion: %v", len(userMap))
			// If the userMap becomes empty after deletion, clean up the map
			if len(userMap) == 0 {
				log.Info().Msgf("UserMap is empty, deleting chatID: %v", chatID)
				delete(db.Data.Workouts, chatID)
			}
		}
	}

	return true
}

func (db *DatabaseManager) GetTotalDistanceByWeek(chatId int64, startDate string, endDate string) (map[int64]string, error) {
	log.Debug().Msgf("Acquiring lock...")
	db.Data.Lock()
	defer db.Data.Unlock()

	if db.Data.Workouts[chatId] == nil {
		log.Warn().Msgf("No workouts found for chat: %v", chatId)
		return nil, fmt.Errorf("no workouts found for chat: %v", chatId)
	}

	totalDistance := make(map[int64]string)

	for userID, userWorkouts := range db.Data.Workouts[chatId] {
		var distance float64
		for date, workout := range userWorkouts {
			if date >= startDate && date <= endDate {
				log.Debug().Msgf("Adding distance to float: %v", workout.Distance)
				floatValue, err := strconv.ParseFloat(workout.Distance, 64)
				if err != nil {
					log.Warn().Msgf("Error parsing distance: %v", err)
					return nil, fmt.Errorf("error parsing distance: %v", err)
				}

				log.Debug().Msgf("Adding distance: %v", workout.Distance)
				distance += floatValue
			}
		}
		totalDistance[userID] = fmt.Sprintf("%.2f", distance)
	}

	return totalDistance, nil
}

func (db *DatabaseManager) GetTotalDistanceByMonth(chatId int64, month string, year string) (map[int64]string, error) {
	log.Debug().Msgf("Acquiring lock...")
	db.Data.Lock()
	defer db.Data.Unlock()

	if db.Data.Workouts[chatId] == nil {
		log.Warn().Msgf("No workouts found for chat: %v", chatId)
		return nil, fmt.Errorf("no workouts found for chat: %v", chatId)
	}

	totalDistance := make(map[int64]string)

	log.Debug().Msgf("Month: %v, Year: %v", month, year)
	for userID, userWorkouts := range db.Data.Workouts[chatId] {
		var distance float64
		for date, workout := range userWorkouts {
			log.Debug().Msgf("date: %v", date)
			log.Debug().Msgf("month: %v, year: %v", date[5:6], date[0:3])
			if date[5:7] == month && date[0:4] == year {
				log.Debug().Msgf("Adding distance to float: %v", workout.Distance)
				floatValue, err := strconv.ParseFloat(workout.Distance, 64)
				if err != nil {
					log.Warn().Msgf("Error parsing distance: %v", err)
					return nil, fmt.Errorf("error parsing distance: %v", err)
				}

				log.Debug().Msgf("Adding distance: %v", workout.Distance)
				distance += floatValue
			}
		}
		totalDistance[userID] = fmt.Sprintf("%.2f", distance)
	}

	log.Debug().Msgf("Releasing Lock...")
	return totalDistance, nil
}

func (db *DatabaseManager) SaveUser(userName string, userId int64) error {
	log.Debug().Msgf("Acquiring lock...")
	db.UserData.Lock()

	// Check if the map is nil and initialize it if necessary
	if db.UserData.Users == nil {
		db.UserData.Users = make(map[int64]string)
	}

	if _, exist := db.UserData.Users[userId]; exist {
		log.Warn().Msgf("User already exists: %v", userName)
		return fmt.Errorf("user already exists: %v", userName)
	}

	log.Info().Msgf("Adding username for userId: %v - %v", userName, userId)
	db.UserData.Users[userId] = userName

	log.Debug().Msgf("Writing into database")
	db.SaveUserData()
	log.Debug().Msgf("Releasing lock...")
	return nil
}

func (db *DatabaseManager) GetUsernameFromId(userId int64) (string, error) {
	value, exist := db.UserData.Users[userId]
	if exist {
		return value, nil
	} else {
		return "", fmt.Errorf("user not found")
	}
}

func (db *DatabaseManager) IsAuthorizedUser(userId int64) bool {
	_, exist := db.UserData.Users[userId]
	if !exist {
		log.Warn().Msgf("User not authorized: %v", userId)
	}
	return exist
}
