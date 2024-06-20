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

func cleanDistanceData(distance string) string {
	log.Debug().Msgf("Cleaning distance data: %s", distance)

	distance = strings.TrimSuffix(distance, ",")
	log.Debug().Msgf("Distance after trimming: %s", distance)

	re := regexp.MustCompile(`[a-zA-Z]+`)

	distance = re.ReplaceAllString(distance, "")
	log.Debug().Msgf("After replacing strings: %s", distance)

	return distance
}
