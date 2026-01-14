package student

import (
	"testing"
)

func TestCalculateNutritionalStatusByMonth_WHOThresholds(t *testing.T) {
	// Test cases based on WHO official BMI-for-age reference data
	// Source: bmi-boys-z-who-2007-exp.xlsx, bmi-girls-z-who-2007-exp.xlsx
	tests := []struct {
		name      string
		gender    Gender
		ageMonths int
		bmi       float32
		expected  NutritionalStatus
	}{
		// Male threshold boundary tests
		{"male 119mo severely wasted", Male, 119, 12.8, SeverelyWasted},
		{"male 119mo wasted", Male, 119, 13.7, Wasted},
		{"male 119mo normal", Male, 119, 13.8, Normal},
		{"male 131mo wasted boundary", Male, 131, 14.0, Wasted},
		{"male 131mo normal boundary", Male, 131, 14.1, Normal},
		{"male 144mo wasted boundary", Male, 144, 14.5, Wasted},
		{"male 144mo normal boundary", Male, 144, 14.6, Normal},

		// Female threshold boundary tests
		{"female 119mo severely wasted", Female, 119, 12.4, SeverelyWasted},
		{"female 119mo wasted", Female, 119, 13.4, Wasted},
		{"female 119mo normal", Female, 119, 13.5, Normal},

		// Edge cases for age range
		{"male min age 61mo normal", Male, 61, 15.0, Normal},
		{"male max age 228mo normal", Male, 228, 18.0, Normal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CalculateNutritionalStatusByMonth(tt.gender, tt.ageMonths, tt.bmi)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if got != tt.expected {
				t.Errorf("CalculateNutritionalStatusByMonth(%s, %d, %.2f) = %v, want %v",
					tt.gender, tt.ageMonths, tt.bmi, got, tt.expected)
			}
		})
	}
}

func TestCalculateNutritionalStatusByMonth_FlaggedStudents(t *testing.T) {
	// Test cases from the 11 flagged students at Timoteo Paez Elementary School
	// These verify the fix for WHO threshold alignment
	tests := []struct {
		name      string
		gender    Gender
		ageMonths int
		bmi       float32
		expected  NutritionalStatus
	}{
		{"Row 5 JOVILLANO", Male, 143, 15.58, Normal},
		{"Row 6 VILLALUZ", Male, 142, 15.46, Normal},
		{"Row 8 LARINAY", Male, 137, 13.99, Wasted},
		{"Row 10 PILAPIL", Male, 131, 13.95, Wasted},
		{"Row 12 GOLOBIO", Male, 147, 14.54, Wasted},
		{"Row 16 TABARES", Male, 136, 14.09, Wasted},
		{"Row 18 PALOMA", Male, 138, 14.77, Normal},
		{"Row 22 ABLOLA", Male, 144, 14.48, Wasted},
		{"Row 25 TUPAS", Male, 148, 14.60, Wasted},
		{"Row 41 GAMBAN", Male, 119, 13.67, Wasted},
		{"Row 42 PALIS", Male, 121, 13.79, Wasted},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CalculateNutritionalStatusByMonth(tt.gender, tt.ageMonths, tt.bmi)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if got != tt.expected {
				t.Errorf("CalculateNutritionalStatusByMonth(%s, %d, %.2f) = %v, want %v",
					tt.gender, tt.ageMonths, tt.bmi, got, tt.expected)
			}
		})
	}
}

func TestCalculateNutritionalStatusByMonth_ValidationErrors(t *testing.T) {
	tests := []struct {
		name      string
		gender    Gender
		ageMonths int
		bmi       float32
	}{
		{"age too low", Male, 60, 15.0},
		{"age too high", Male, 229, 15.0},
		{"invalid gender", Gender("unknown"), 100, 15.0},
		{"zero bmi", Male, 100, 0},
		{"negative bmi", Male, 100, -1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CalculateNutritionalStatusByMonth(tt.gender, tt.ageMonths, tt.bmi)
			if err == nil {
				t.Errorf("expected error for %s, got nil", tt.name)
			}
		})
	}
}

func TestWHOThresholdValues(t *testing.T) {
	// Verify specific WHO threshold values are correctly loaded
	// These are spot-checks against the official WHO reference data
	tests := []struct {
		name              string
		gender            Gender
		ageMonths         int
		expectedSevWasted float32
		expectedWasted    float32
	}{
		// Male thresholds from WHO bmi-boys-z-who-2007-exp.xlsx
		{"male 61mo", Male, 61, 12.1, 13.0},
		{"male 119mo", Male, 119, 12.8, 13.7},
		{"male 131mo", Male, 131, 13.0, 14.0},
		{"male 144mo", Male, 144, 13.4, 14.5},
		{"male 228mo", Male, 228, 15.9, 17.6},

		// Female thresholds from WHO bmi-girls-z-who-2007-exp.xlsx
		{"female 61mo", Female, 61, 11.8, 12.7},
		{"female 119mo", Female, 119, 12.4, 13.4},
		{"female 144mo", Female, 144, 13.2, 14.4},
		{"female 228mo", Female, 228, 14.7, 16.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var thresholds [2]float32
			var exists bool

			if tt.gender == Male {
				thresholds, exists = maleBMIThresholdsByMonth[tt.ageMonths]
			} else {
				thresholds, exists = femaleBMIThresholdsByMonth[tt.ageMonths]
			}

			if !exists {
				t.Errorf("no thresholds found for %s age %d", tt.gender, tt.ageMonths)
				return
			}

			if thresholds[0] != tt.expectedSevWasted {
				t.Errorf("severely wasted threshold for %s age %d = %.1f, want %.1f",
					tt.gender, tt.ageMonths, thresholds[0], tt.expectedSevWasted)
			}

			if thresholds[1] != tt.expectedWasted {
				t.Errorf("wasted threshold for %s age %d = %.1f, want %.1f",
					tt.gender, tt.ageMonths, thresholds[1], tt.expectedWasted)
			}
		})
	}
}

func TestNutritionalStatusString(t *testing.T) {
	tests := []struct {
		status   NutritionalStatus
		expected string
	}{
		{Normal, "Normal"},
		{Wasted, "Wasted"},
		{SeverelyWasted, "Severely Wasted"},
		{NutritionalStatusError, "Error"},
		{NutritionalStatusGenderError, "Gender Error"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.status.String(); got != tt.expected {
				t.Errorf("NutritionalStatus.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}
