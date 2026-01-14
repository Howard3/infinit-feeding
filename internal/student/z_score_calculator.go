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

// Monthly BMI thresholds: map[month][2]float32{severelyWasted (-3SD), wasted (-2SD)}
// Source: WHO BMI-for-age reference data (bmi-boys-z-who-2007-exp.xlsx, bmi-girls-z-who-2007-exp.xlsx)
// Downloaded from: https://www.who.int/tools/growth-reference-data-for-5to19-years/indicators/bmi-for-age
var maleBMIThresholdsByMonth = map[int][2]float32{
	61: {12.1, 13.0}, 62: {12.1, 13.0}, 63: {12.1, 13.0}, 64: {12.1, 13.0}, 65: {12.1, 13.0},
	66: {12.1, 13.0}, 67: {12.1, 13.0}, 68: {12.1, 13.0}, 69: {12.1, 13.0}, 70: {12.1, 13.0},
	71: {12.1, 13.0}, 72: {12.1, 13.0}, 73: {12.1, 13.0}, 74: {12.2, 13.1}, 75: {12.2, 13.1},
	76: {12.2, 13.1}, 77: {12.2, 13.1}, 78: {12.2, 13.1}, 79: {12.2, 13.1}, 80: {12.2, 13.1},
	81: {12.2, 13.1}, 82: {12.2, 13.1}, 83: {12.2, 13.1}, 84: {12.2, 13.1}, 85: {12.3, 13.2},
	86: {12.3, 13.2}, 87: {12.3, 13.2}, 88: {12.3, 13.2}, 89: {12.3, 13.2}, 90: {12.3, 13.2},
	91: {12.3, 13.2}, 92: {12.3, 13.2}, 93: {12.4, 13.3}, 94: {12.4, 13.3}, 95: {12.4, 13.3},
	96: {12.4, 13.3}, 97: {12.4, 13.3}, 98: {12.4, 13.3}, 99: {12.4, 13.3}, 100: {12.4, 13.4},
	101: {12.5, 13.4}, 102: {12.5, 13.4}, 103: {12.5, 13.4}, 104: {12.5, 13.4}, 105: {12.5, 13.4},
	106: {12.5, 13.5}, 107: {12.5, 13.5}, 108: {12.6, 13.5}, 109: {12.6, 13.5}, 110: {12.6, 13.5},
	111: {12.6, 13.5}, 112: {12.6, 13.6}, 113: {12.6, 13.6}, 114: {12.7, 13.6}, 115: {12.7, 13.6},
	116: {12.7, 13.6}, 117: {12.7, 13.7}, 118: {12.7, 13.7}, 119: {12.8, 13.7}, 120: {12.8, 13.7},
	121: {12.8, 13.8}, 122: {12.8, 13.8}, 123: {12.8, 13.8}, 124: {12.9, 13.8}, 125: {12.9, 13.9},
	126: {12.9, 13.9}, 127: {12.9, 13.9}, 128: {13.0, 13.9}, 129: {13.0, 14.0}, 130: {13.0, 14.0},
	131: {13.0, 14.0}, 132: {13.1, 14.1}, 133: {13.1, 14.1}, 134: {13.1, 14.1}, 135: {13.1, 14.1},
	136: {13.2, 14.2}, 137: {13.2, 14.2}, 138: {13.2, 14.2}, 139: {13.2, 14.3}, 140: {13.3, 14.3},
	141: {13.3, 14.3}, 142: {13.3, 14.4}, 143: {13.4, 14.4}, 144: {13.4, 14.5}, 145: {13.4, 14.5},
	146: {13.5, 14.5}, 147: {13.5, 14.6}, 148: {13.5, 14.6}, 149: {13.6, 14.6}, 150: {13.6, 14.7},
	151: {13.6, 14.7}, 152: {13.7, 14.8}, 153: {13.7, 14.8}, 154: {13.7, 14.8}, 155: {13.8, 14.9},
	156: {13.8, 14.9}, 157: {13.8, 15.0}, 158: {13.9, 15.0}, 159: {13.9, 15.1}, 160: {14.0, 15.1},
	161: {14.0, 15.2}, 162: {14.0, 15.2}, 163: {14.1, 15.2}, 164: {14.1, 15.3}, 165: {14.1, 15.3},
	166: {14.2, 15.4}, 167: {14.2, 15.4}, 168: {14.3, 15.5}, 169: {14.3, 15.5}, 170: {14.3, 15.6},
	171: {14.4, 15.6}, 172: {14.4, 15.7}, 173: {14.5, 15.7}, 174: {14.5, 15.7}, 175: {14.5, 15.8},
	176: {14.6, 15.8}, 177: {14.6, 15.9}, 178: {14.6, 15.9}, 179: {14.7, 16.0}, 180: {14.7, 16.0},
	181: {14.7, 16.1}, 182: {14.8, 16.1}, 183: {14.8, 16.1}, 184: {14.8, 16.2}, 185: {14.9, 16.2},
	186: {14.9, 16.3}, 187: {14.9, 16.3}, 188: {15.0, 16.3}, 189: {15.0, 16.4}, 190: {15.0, 16.4},
	191: {15.1, 16.5}, 192: {15.1, 16.5}, 193: {15.1, 16.5}, 194: {15.2, 16.6}, 195: {15.2, 16.6},
	196: {15.2, 16.7}, 197: {15.3, 16.7}, 198: {15.3, 16.7}, 199: {15.3, 16.8}, 200: {15.3, 16.8},
	201: {15.4, 16.8}, 202: {15.4, 16.9}, 203: {15.4, 16.9}, 204: {15.4, 16.9}, 205: {15.5, 17.0},
	206: {15.5, 17.0}, 207: {15.5, 17.0}, 208: {15.5, 17.1}, 209: {15.6, 17.1}, 210: {15.6, 17.1},
	211: {15.6, 17.1}, 212: {15.6, 17.2}, 213: {15.6, 17.2}, 214: {15.7, 17.2}, 215: {15.7, 17.3},
	216: {15.7, 17.3}, 217: {15.7, 17.3}, 218: {15.7, 17.3}, 219: {15.7, 17.4}, 220: {15.8, 17.4},
	221: {15.8, 17.4}, 222: {15.8, 17.4}, 223: {15.8, 17.5}, 224: {15.8, 17.5}, 225: {15.8, 17.5},
	226: {15.8, 17.5}, 227: {15.8, 17.5}, 228: {15.9, 17.6},
}

var femaleBMIThresholdsByMonth = map[int][2]float32{
	61: {11.8, 12.7}, 62: {11.8, 12.7}, 63: {11.8, 12.7}, 64: {11.8, 12.7}, 65: {11.7, 12.7},
	66: {11.7, 12.7}, 67: {11.7, 12.7}, 68: {11.7, 12.7}, 69: {11.7, 12.7}, 70: {11.7, 12.7},
	71: {11.7, 12.7}, 72: {11.7, 12.7}, 73: {11.7, 12.7}, 74: {11.7, 12.7}, 75: {11.7, 12.7},
	76: {11.7, 12.7}, 77: {11.7, 12.7}, 78: {11.7, 12.7}, 79: {11.7, 12.7}, 80: {11.7, 12.7},
	81: {11.7, 12.7}, 82: {11.7, 12.7}, 83: {11.7, 12.7}, 84: {11.8, 12.7}, 85: {11.8, 12.7},
	86: {11.8, 12.8}, 87: {11.8, 12.8}, 88: {11.8, 12.8}, 89: {11.8, 12.8}, 90: {11.8, 12.8},
	91: {11.8, 12.8}, 92: {11.8, 12.8}, 93: {11.8, 12.8}, 94: {11.9, 12.9}, 95: {11.9, 12.9},
	96: {11.9, 12.9}, 97: {11.9, 12.9}, 98: {11.9, 12.9}, 99: {11.9, 12.9}, 100: {11.9, 13.0},
	101: {12.0, 13.0}, 102: {12.0, 13.0}, 103: {12.0, 13.0}, 104: {12.0, 13.0}, 105: {12.0, 13.1},
	106: {12.1, 13.1}, 107: {12.1, 13.1}, 108: {12.1, 13.1}, 109: {12.1, 13.2}, 110: {12.1, 13.2},
	111: {12.2, 13.2}, 112: {12.2, 13.2}, 113: {12.2, 13.3}, 114: {12.2, 13.3}, 115: {12.3, 13.3},
	116: {12.3, 13.4}, 117: {12.3, 13.4}, 118: {12.3, 13.4}, 119: {12.4, 13.4}, 120: {12.4, 13.5},
	121: {12.4, 13.5}, 122: {12.4, 13.5}, 123: {12.5, 13.6}, 124: {12.5, 13.6}, 125: {12.5, 13.6},
	126: {12.5, 13.7}, 127: {12.6, 13.7}, 128: {12.6, 13.7}, 129: {12.6, 13.8}, 130: {12.7, 13.8},
	131: {12.7, 13.8}, 132: {12.7, 13.9}, 133: {12.8, 13.9}, 134: {12.8, 14.0}, 135: {12.8, 14.0},
	136: {12.9, 14.0}, 137: {12.9, 14.1}, 138: {12.9, 14.1}, 139: {13.0, 14.2}, 140: {13.0, 14.2},
	141: {13.0, 14.3}, 142: {13.1, 14.3}, 143: {13.1, 14.3}, 144: {13.2, 14.4}, 145: {13.2, 14.4},
	146: {13.2, 14.5}, 147: {13.3, 14.5}, 148: {13.3, 14.6}, 149: {13.3, 14.6}, 150: {13.4, 14.7},
	151: {13.4, 14.7}, 152: {13.5, 14.8}, 153: {13.5, 14.8}, 154: {13.5, 14.8}, 155: {13.6, 14.9},
	156: {13.6, 14.9}, 157: {13.6, 15.0}, 158: {13.7, 15.0}, 159: {13.7, 15.1}, 160: {13.8, 15.1},
	161: {13.8, 15.2}, 162: {13.8, 15.2}, 163: {13.9, 15.2}, 164: {13.9, 15.3}, 165: {13.9, 15.3},
	166: {14.0, 15.4}, 167: {14.0, 15.4}, 168: {14.0, 15.4}, 169: {14.1, 15.5}, 170: {14.1, 15.5},
	171: {14.1, 15.6}, 172: {14.1, 15.6}, 173: {14.2, 15.6}, 174: {14.2, 15.7}, 175: {14.2, 15.7},
	176: {14.3, 15.7}, 177: {14.3, 15.8}, 178: {14.3, 15.8}, 179: {14.3, 15.8}, 180: {14.4, 15.9},
	181: {14.4, 15.9}, 182: {14.4, 15.9}, 183: {14.4, 16.0}, 184: {14.4, 16.0}, 185: {14.5, 16.0},
	186: {14.5, 16.0}, 187: {14.5, 16.1}, 188: {14.5, 16.1}, 189: {14.5, 16.1}, 190: {14.6, 16.1},
	191: {14.6, 16.2}, 192: {14.6, 16.2}, 193: {14.6, 16.2}, 194: {14.6, 16.2}, 195: {14.6, 16.2},
	196: {14.6, 16.2}, 197: {14.6, 16.3}, 198: {14.7, 16.3}, 199: {14.7, 16.3}, 200: {14.7, 16.3},
	201: {14.7, 16.3}, 202: {14.7, 16.3}, 203: {14.7, 16.3}, 204: {14.7, 16.4}, 205: {14.7, 16.4},
	206: {14.7, 16.4}, 207: {14.7, 16.4}, 208: {14.7, 16.4}, 209: {14.7, 16.4}, 210: {14.7, 16.4},
	211: {14.7, 16.4}, 212: {14.7, 16.4}, 213: {14.7, 16.4}, 214: {14.7, 16.4}, 215: {14.7, 16.4},
	216: {14.7, 16.4}, 217: {14.7, 16.5}, 218: {14.7, 16.5}, 219: {14.7, 16.5}, 220: {14.7, 16.5},
	221: {14.7, 16.5}, 222: {14.7, 16.5}, 223: {14.7, 16.5}, 224: {14.7, 16.5}, 225: {14.7, 16.5},
	226: {14.7, 16.5}, 227: {14.7, 16.5}, 228: {14.7, 16.5},
}

// CalculateNutritionalStatusByMonth determines the nutritional status of a student
// based on their gender, age in months, and BMI using WHO monthly thresholds.
func CalculateNutritionalStatusByMonth(gender Gender, ageMonths int, bmi float32) (NutritionalStatus, error) {
	if ageMonths < 61 || ageMonths > 228 {
		return NutritionalStatusError, fmt.Errorf("age must be between 61 and 228 months (5-19 years), got %d", ageMonths)
	}

	if gender != Male && gender != Female {
		return NutritionalStatusError, fmt.Errorf("gender must be either 'male' or 'female', got %s", gender)
	}

	if bmi <= 0 {
		return NutritionalStatusError, fmt.Errorf("BMI must be positive, got %f", bmi)
	}

	var thresholds [2]float32
	var exists bool

	if gender == Male {
		thresholds, exists = maleBMIThresholdsByMonth[ageMonths]
	} else {
		thresholds, exists = femaleBMIThresholdsByMonth[ageMonths]
	}

	if !exists {
		return NutritionalStatusError, fmt.Errorf("no thresholds found for gender %s and age %d months", gender, ageMonths)
	}

	severelyWastedThreshold := thresholds[0]
	wastedThreshold := thresholds[1]

	if bmi <= severelyWastedThreshold {
		return SeverelyWasted, nil
	} else if bmi <= wastedThreshold {
		return Wasted, nil
	}
	return Normal, nil
}

// Legacy yearly thresholds kept for reference
var bmiThresholds = map[Gender]map[int][2]float32{
	Female: {
		5: {13.9, 13.0}, 6: {13.8, 12.9}, 7: {13.7, 12.9}, 8: {13.7, 12.9}, 9: {13.8, 13.0},
		10: {14.0, 13.2}, 11: {14.3, 13.5}, 12: {14.8, 14.0}, 13: {15.3, 14.5}, 14: {15.8, 15.0},
		15: {16.3, 15.5}, 16: {16.7, 15.9}, 17: {17.0, 16.2}, 18: {17.2, 16.4}, 19: {17.2, 16.4},
	},
	Male: {
		5: {13.8, 12.9}, 6: {13.7, 12.8}, 7: {13.6, 12.7}, 8: {13.5, 12.6}, 9: {13.5, 12.6},
		10: {13.6, 12.7}, 11: {13.9, 12.9}, 12: {14.3, 13.3}, 13: {14.8, 13.8}, 14: {15.3, 14.3},
		15: {15.8, 14.8}, 16: {16.3, 15.3}, 17: {16.7, 15.7}, 18: {17.0, 16.0}, 19: {17.3, 16.3},
	},
}

// CalculateNutritionalStatus determines the nutritional status using yearly thresholds.
// Deprecated: Use CalculateNutritionalStatusByMonth for WHO-compatible calculations.
func CalculateNutritionalStatus(gender Gender, age int, bmi float32) (NutritionalStatus, error) {
	if age < 5 || age > 19 {
		return 0, fmt.Errorf("age must be between 5 and 19, got %d", age)
	}

	if gender != Male && gender != Female {
		return 0, fmt.Errorf("gender must be either 'male' or 'female', got %s", gender)
	}

	if bmi <= 0 {
		return 0, fmt.Errorf("BMI must be positive, got %f", bmi)
	}

	thresholds, exists := bmiThresholds[gender][age]
	if !exists {
		return 0, fmt.Errorf("no thresholds found for gender %s and age %d", gender, age)
	}

	wastedThreshold := thresholds[0]
	severelyWastedThreshold := thresholds[1]

	if bmi < severelyWastedThreshold {
		return SeverelyWasted, nil
	} else if bmi < wastedThreshold {
		return Wasted, nil
	}
	return Normal, nil
}
