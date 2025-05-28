package student

import "fmt"

// NutritionalStatus represents the classification of a student's nutritional status
// based on their BMI z-score
type NutritionalStatus int

const (
	// SeverelyWasted indicates BMI is below the -3 SD threshold
	SeverelyWasted NutritionalStatus = iota
	// Wasted indicates BMI is below the -2 SD but above the -3 SD threshold
	Wasted
	// Normal indicates BMI is at or above the -2 SD threshold
	Normal
	// Error indicates an error occurred during calculation
	NutritionalStatusError
	NutritionalStatusGenderError
)

// String returns the string representation of the nutritional status
func (ns NutritionalStatus) String() string {
	switch ns {
	case Normal:
		return "Normal"
	case Wasted:
		return "Wasted"
	case SeverelyWasted:
		return "Severely Wasted"
	case NutritionalStatusError:
		return "Error"
	case NutritionalStatusGenderError:
		return "Gender Error"
	default:
		return "Unknown"
	}
}

// Gender represents the gender of a student
type Gender string

const (
	Male   Gender = "male"
	Female Gender = "female"
)

// bmiThresholds contains the lower thresholds for BMI classifications by gender and age
// The first threshold is for -2 SD (wasted) and the second is for -3 SD (severely wasted)
var bmiThresholds = map[Gender]map[int][2]float32{
	Female: {
		5:  {13.9, 13.0},
		6:  {13.8, 12.9},
		7:  {13.7, 12.9},
		8:  {13.7, 12.9},
		9:  {13.8, 13.0},
		10: {14.0, 13.2},
		11: {14.3, 13.5},
		12: {14.8, 14.0},
		13: {15.3, 14.5},
		14: {15.8, 15.0},
		15: {16.3, 15.5},
		16: {16.7, 15.9},
		17: {17.0, 16.2},
		18: {17.2, 16.4},
		19: {17.2, 16.4},
	},
	Male: {
		5:  {13.8, 12.9},
		6:  {13.7, 12.8},
		7:  {13.6, 12.7},
		8:  {13.5, 12.6},
		9:  {13.5, 12.6},
		10: {13.6, 12.7},
		11: {13.9, 12.9},
		12: {14.3, 13.3},
		13: {14.8, 13.8},
		14: {15.3, 14.3},
		15: {15.8, 14.8},
		16: {16.3, 15.3},
		17: {16.7, 15.7},
		18: {17.0, 16.0},
		19: {17.3, 16.3},
	},
}

// normalBmiUpperThresholds contains the upper thresholds for normal BMI by gender and age
var normalBmiUpperThresholds = map[Gender]map[int]float32{
	Female: {
		5:  17.0,
		6:  17.1,
		7:  17.3,
		8:  17.8,
		9:  18.4,
		10: 19.0,
		11: 19.7,
		12: 20.4,
		13: 21.1,
		14: 21.8,
		15: 22.5,
		16: 23.2,
		17: 23.7,
		18: 24.0,
		19: 24.2,
	},
	Male: {
		5:  17.0,
		6:  17.2,
		7:  17.4,
		8:  17.9,
		9:  18.4,
		10: 19.0,
		11: 19.6,
		12: 20.3,
		13: 20.9,
		14: 21.5,
		15: 22.1,
		16: 22.6,
		17: 23.0,
		18: 23.3,
		19: 23.6,
	},
}

// CalculateNutritionalStatus determines the nutritional status of a student
// based on their gender, age, and BMI.
// Age should be in years (5-19) and BMI is calculated as weight(kg) / height(m)².
func CalculateNutritionalStatus(gender Gender, age int, bmi float32) (NutritionalStatus, error) {
	// Validate input parameters
	if age < 5 || age > 19 {
		return 0, fmt.Errorf("age must be between 5 and 19, got %d", age)
	}

	if gender != Male && gender != Female {
		return 0, fmt.Errorf("gender must be either 'male' or 'female', got %s", gender)
	}

	if bmi <= 0 {
		return 0, fmt.Errorf("BMI must be positive, got %f", bmi)
	}

	// Get thresholds for the specified gender and age
	thresholds, exists := bmiThresholds[gender][age]
	if !exists {
		return 0, fmt.Errorf("no thresholds found for gender %s and age %d", gender, age)
	}

	wastedThreshold := thresholds[0]
	severelyWastedThreshold := thresholds[1]

	// Determine nutritional status based on BMI
	if bmi < severelyWastedThreshold {
		return SeverelyWasted, nil
	} else if bmi < wastedThreshold {
		return Wasted, nil
	} else {
		return Normal, nil
	}
}

// IsBMIInNormalRange checks if the given BMI is within the normal range for the specified gender and age
func IsBMIInNormalRange(gender Gender, age int, bmi float32) (bool, error) {
	// Validate input parameters
	if age < 5 || age > 19 {
		return false, fmt.Errorf("age must be between 5 and 19, got %d", age)
	}

	if gender != Male && gender != Female {
		return false, fmt.Errorf("gender must be either 'male' or 'female', got %s", gender)
	}

	// Get lower threshold
	lowerThresholds, exists := bmiThresholds[gender][age]
	if !exists {
		return false, fmt.Errorf("no thresholds found for gender %s and age %d", gender, age)
	}

	// Get upper threshold
	upperThreshold, exists := normalBmiUpperThresholds[gender][age]
	if !exists {
		return false, fmt.Errorf("no upper threshold found for gender %s and age %d", gender, age)
	}

	// Check if BMI is within normal range
	return bmi >= lowerThresholds[0] && bmi <= upperThreshold, nil
}
