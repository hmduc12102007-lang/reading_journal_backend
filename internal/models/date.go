package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

const dateLayout = "2006-01-02"

type Date struct {
	time.Time
}

func NewDate(value time.Time) Date {
	return Date{Time: time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)}
}

func ParseDate(value string) (Date, error) {
	parsed, err := time.Parse(dateLayout, value)
	if err != nil {
		return Date{}, err
	}
	return NewDate(parsed), nil
}

func (d Date) Value() (driver.Value, error) {
	if d.IsZero() {
		return nil, nil
	}
	return d.Format(dateLayout), nil
}

func (d *Date) Scan(value interface{}) error {
	switch value := value.(type) {
	case time.Time:
		d.Time = NewDate(value).Time
	case []byte:
		parsed, err := ParseDate(string(value))
		if err != nil {
			return err
		}
		d.Time = parsed.Time
	case string:
		parsed, err := ParseDate(value)
		if err != nil {
			return err
		}
		d.Time = parsed.Time
	case nil:
		d.Time = time.Time{}
	default:
		return fmt.Errorf("cannot scan %T into models.Date", value)
	}
	return nil
}

func (Date) GormDataType() string {
	return "date"
}

func (d Date) MarshalJSON() ([]byte, error) {
	if d.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(d.Format(dateLayout))
}

func (d *Date) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		d.Time = time.Time{}
		return nil
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	parsed, err := ParseDate(value)
	if err != nil {
		return err
	}
	d.Time = parsed.Time
	return nil
}
