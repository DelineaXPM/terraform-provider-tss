package delinea

import (
	"fmt"
	"strconv"
)

const (
	minServerInt = -1 << 31
	maxServerInt = 1<<31 - 1
)

func toServerInt(value int64, field string) (int, error) {
	if value < minServerInt || value > maxServerInt {
		return 0, fmt.Errorf("%s value %d is outside Secret Server's supported 32-bit integer range", field, value)
	}
	return int(value), nil
}

func toPositiveServerInt(value int64, field string) (int, error) {
	converted, err := toServerInt(value, field)
	if err != nil {
		return 0, err
	}
	if converted <= 0 {
		return 0, fmt.Errorf("%s must be a positive Secret Server ID", field)
	}
	return converted, nil
}

func parsePositiveServerInt(value, field string) (int, error) {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", field, err)
	}
	return toPositiveServerInt(parsed, field)
}
