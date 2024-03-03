package webapi

import (
	"fmt"
	"geevly/gen/go/eda"
	"time"

	"github.com/Howard3/valueextractor"
)

func AsProtoDate(ref *eda.Date) valueextractor.Converter {
	return func(ec *valueextractor.Extractor, value string) error {
		date, err := time.Parse("2006-01-02", value)
		if err != nil {
			return fmt.Errorf("invalid date format")
		}

		*ref = eda.Date{Year: int32(date.Year()), Month: int32(date.Month()), Day: int32(date.Day())}

		return nil
	}
}
