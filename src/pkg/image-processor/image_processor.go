package imageprocessor

import (
	"os"
	"regexp"
	"run-tracker-telebot/src/log"

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

	// file, err := os.Open(imagePath)
	// if err != nil {
	// 	log.Warn().Msgf("Unable to open image: %v", err)
	// 	return "", err
	// }

	// defer file.Close()

	// log.Debug().Msgf("Decoding image...")
	// img, _, err := image.Decode(file)
	// if err != nil {
	// 	log.Warn().Msgf("Unable to decode image: %v", err)
	// 	return "", err
	// }

	// buffer := new(bytes.Buffer)
	// if err := jpeg.Encode(buffer, img, nil); err != nil {
	// 	log.Warn().Msgf("Unable to encode image.")
	// }

	// // Perform OCR on the image
	// client.SetImageFromBytes(buffer.Bytes())
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
	dateRegex := regexp.MustCompile(`(?m)Fri, \d{2} \w{3}`)
	distanceRegex := regexp.MustCompile(`\d+\.\d+[a-zA-Z]+`)
	paceRegex := regexp.MustCompile(`\d{1,2}'\d{2}"/[a-zA-Z]+`)
	heartRateRegex := regexp.MustCompile(`\d{2,3}BPM`)

	// Extract details using regular expressions
	date := dateRegex.FindString(text)
	distance := distanceRegex.FindString(text)
	pace := paceRegex.FindString(text)
	heartRate := heartRateRegex.FindString(text)

	// Store extracted details in a map
	workoutDetails := map[string]string{
		"Date":      date,
		"Distance":  distance,
		"Pace":      pace,
		"HeartRate": heartRate,
	}

	return workoutDetails, nil
}
