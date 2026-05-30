package goodglobalstate

import (
	"errors"
	"regexp"
)

const schemaJSON = `{"type":"object"}`

var (
	errNotFound = errors.New("not found")
	nameRe      = regexp.MustCompile(`^[a-z]+$`)
)

func rankSeverity(value string) int {
	switch value {
	case "critical":
		return 3
	case "warning":
		return 2
	case "info":
		return 1
	default:
		return 0
	}
}

func validateName(value string) bool {
	return nameRe.MatchString(value)
}

func sentinel() error {
	return errNotFound
}
