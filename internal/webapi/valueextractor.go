package webapi

import (
	"fmt"
	"geevly/gen/go/eda"
	"time"

	vex "github.com/Howard3/valueextractor"
)

func AsProtoDate(ref *eda.Date) vex.Converter {
	return func(ec *vex.Extractor, value string) error {
		date, err := time.Parse("2006-01-02", value)
		if err != nil {
			return fmt.Errorf("invalid date format")
		}

		*ref = eda.Date{Year: int32(date.Year()), Month: int32(date.Month()), Day: int32(date.Day())}

		return nil
	}
}

func ReturnProtoDate(ec *vex.Extractor, key string) *eda.Date {
	var date eda.Date
	ec.With(key, AsProtoDate(&date))
	return &date
}
