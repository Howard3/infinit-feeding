# BMI Z-Score Calculator

## Overview

This package provides a BMI Z-Score calculator for assessing nutritional status in children and adolescents aged 5-19 years based on WHO growth reference standards. The calculator determines if a child's BMI falls into categories of Normal, Wasted, or Severely Wasted based on their gender, age, and calculated BMI value.

## Background

The WHO Growth Reference for school-aged children and adolescents (5-19 years) provides BMI-for-age standards to assess nutritional status. BMI (Body Mass Index) is calculated as weight(kg) / height(m)². 

Z-scores, or standard deviation scores, indicate how many standard deviations a child's BMI is from the median BMI for their age and gender. In this implementation:

- **Normal**: BMI ≥ -2 SD (Z-score at or above -2)
- **Wasted**: BMI < -2 SD (Z-score below -2)
- **Severely Wasted**: BMI < -3 SD (Z-score below -3)

## Implementation

The calculator is implemented in `z_score_calculator.go` with the following key components:

### Data Types

```go
// NutritionalStatus represents the classification of a student's nutritional status
type NutritionalStatus int

const (
    SeverelyWasted NutritionalStatus = iota
    Wasted
    Normal
)

// Gender represents the gender of a student
type Gender string

const (
    Male   Gender = "male"
    Female Gender = "female"
)
```

### Core Functions

- `CalculateNutritionalStatus(gender Gender, age int, bmi float64) (NutritionalStatus, error)`
  Determines nutritional status based on gender, age, and BMI.

- `IsBMIInNormalRange(gender Gender, age int, bmi float64) (bool, error)`
  Helper function to check if BMI is within the normal range.

## Usage Examples

### Basic Usage

```go
package main

import (
    "fmt"
    "github.com/yourusername/infinit-feeding/internal/student"
)

func main() {
    // Example for a 10-year-old boy with BMI of 13.5
    gender := student.Male
    age := 10
    bmi := 13.5 // Calculated as weight(kg) / height(m)²

    status, err := student.CalculateNutritionalStatus(gender, age, bmi)
    if err != nil {
        fmt.Printf("Error: %v\n", err)
        return
    }
    
    fmt.Printf("Nutritional Status: %s\n", status)
    // Output would be: Nutritional Status: Wasted
    
    // Check if BMI is in normal range
    isNormal, err := student.IsBMIInNormalRange(gender, age, bmi)
    if err != nil {
        fmt.Printf("Error: %v\n", err)
        return
    }
    
    fmt.Printf("Is BMI in normal range: %v\n", isNormal)
    // Output would be: Is BMI in normal range: false
}
```

### Batch Processing

```go
// Process multiple students
students := []struct {
    Gender student.Gender
    Age    int
    BMI    float64
}{
    {student.Female, 7, 13.5},
    {student.Male, 12, 14.2},
    {student.Female, 16, 16.5},
}

for i, s := range students {
    status, err := student.CalculateNutritionalStatus(s.Gender, s.Age, s.BMI)
    if err != nil {
        fmt.Printf("Student %d: Error - %v\n", i+1, err)
        continue
    }
    fmt.Printf("Student %d: %s\n", i+1, status)
}
```

## Supported Age Range

This calculator is designed for children and adolescents aged 5-19 years only. For children under 5 years of age, different growth standards should be used.

## Data Sources

The BMI threshold values are based on the WHO Growth Reference 2007, which provides BMI-for-age tables for children and adolescents aged 5-19 years.

## References

- WHO Growth Reference 2007: [WHO Growth Reference for School-aged Children and Adolescents](https://www.who.int/toolkits/growth-reference-data-for-5to19-years)
- BMI Classification: [WHO BMI-for-age (5-19 years)](https://www.who.int/tools/growth-reference-data-for-5to19-years/indicators/bmi-for-age)