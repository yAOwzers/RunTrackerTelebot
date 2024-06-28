package imageprocessor

import (
	"os"
	"regexp"
	"run-tracker-telebot/src/log"
	"strings"
	"time"

	"github.com/otiai10/gosseract/v2"
)

type ImageProcessor struct{}

func NewImageProcessor() *ImageProcessor {
	return &ImageProcessor{}
}

func (ip *ImageProcessor) ProcessImage(imagePath string) (string, error) {
	log.Info().Msgf("Creating new Tesseract client...")
	client := gosseract.NewClient()
	defer client.Close()

	log.Info().Msgf("Processing image: %s", imagePath)

	_, err := os.Stat(imagePath)
	if err != nil {
		log.Warn().Msgf("Error reading file: %v", err)
		return "", err
	}

	client.SetImage(imagePath)
	text, err := client.Text()
	if err != nil {
		log.Warn().Msgf("Error reading text from image: %v", err)
		return "", err
	}

	log.Debug().Msgf("Text extracted from image: %s", text)

	return text, nil
}

func (ip *ImageProcessor) ParseWorkoutDetails(text string) (map[string]string, error) {
	// Define regular expressions for matching
	distanceRegex := regexp.MustCompile(`\d+\.\d+[a-zA-Z]*`)
	paceRegex := regexp.MustCompile(`\d{1,2}[':]\d{2}(")?/[a-zA-Z]*`)
	// heartRateRegex := regexp.MustCompile(`\d{2,3}[BPM]{0,1}`)

	// Extract details using regular expressions
	distance := distanceRegex.FindString(text)
	pace := paceRegex.FindString(text)
	// heartRate := heartRateRegex.FindString(text)

	log.Debug().Msgf("Distance: %s", distance)
	log.Debug().Msgf("Pace: %s", pace)
	// log.Debug().Msgf("Heart Rate: %s", heartRate)

	if distance == "" || pace == "" {
		log.Warn().Msgf("Error extracting workout details")
		return nil, nil
	}

	distance = cleanDistanceData(distance)

	date := time.Now().Format("2006-01-02")
	// Store extracted details in a map
	workoutDetails := map[string]string{
		"Date":     date,
		"Distance": distance,
		"Pace":     pace,
		// "HeartRate": heartRate,
	}

	return workoutDetails, nil
}

func (ip *ImageProcessor) ParseRunKeepWorkoutDetails(text string) (map[string]string, error) {
	// Define regular expressions for matching
	distanceRegex := regexp.MustCompile(`\b\d+\.\d+\b`)
	paceRegex := regexp.MustCompile(`\b\d{1,2}:\d{2}\b`)
	// timeRegex := regexp.MustCompile(`\b\d{1,2}:\d{2}\b`) // To extract total time
	caloriesRegex := regexp.MustCompile(`\b\d+\b`) // To extract calories

	// Extract details using regular expressions
	distance := distanceRegex.FindString(text)
	timeAndPaceMatches := paceRegex.FindAllString(text, -1)
	calories := caloriesRegex.FindString(text)

	// Ensure the first pace match is not the total time
	if len(timeAndPaceMatches) < 2 || len(calories) < 1 {
		log.Printf("Error extracting workout details")
		return nil, nil
	}

	totalTime := timeAndPaceMatches[1]
	pace := timeAndPaceMatches[0]

	log.Debug().Msgf("Total Time: %s", totalTime)
	log.Debug().Msgf("Distance: %s", distance)
	log.Debug().Msgf("Pace: %s", pace)
	log.Debug().Msgf("Calories: %s", calories)

	if distance == "" || pace == "" {
		log.Warn().Msgf("Error extracting workout details")
		return nil, nil
	}

	// Clean distance data
	distance = cleanDistanceData(distance)

	date := time.Now().Format("2006-01-02")
	// Store extracted details in a map
	workoutDetails := map[string]string{
		"Date":      date,
		"Distance":  distance,
		"TotalTime": totalTime,
		"Pace":      pace,
		"Calories":  calories,
	}

	return workoutDetails, nil
}

func cleanDistanceData(distance string) string {
	log.Debug().Msgf("Cleaning distance data: %s", distance)

	distance = strings.TrimSuffix(distance, ",")
	log.Debug().Msgf("Distance after trimming: %s", distance)

	re := regexp.MustCompile(`[a-zA-Z]+`)

	distance = re.ReplaceAllString(distance, "")
	log.Debug().Msgf("After replacing strings: %s", distance)

	return distance
}

func (ip *ImageProcessor) IsAppleWorkout(text string) bool {
	keywords := []string{
		"Workout", "Time", "Distance",
		"Active Kilocalories", "Total Kilocalories",
	}

	for _, keyword := range keywords {
		if !strings.Contains(text, keyword) {
			log.Debug().Msgf("Keyword not found in Text: %v", keyword)
			log.Debug().Msgf("Not Apple Workout")
			return false
		}
	}
	return true
}

func (ip *ImageProcessor) IsRunKeeper(text string) bool {
	keywords := []string{
		"km", "time", "min/km", "Calories",
	}

	for _, keyword := range keywords {
		if !strings.Contains(text, keyword) {
			log.Debug().Msgf("Keyword not found in Text: %v", keyword)
			log.Debug().Msgf("Not Run Keeper")
			return false
		}
	}
	return true
}
