package proto

import (
	"fmt"
	"regexp"
)

const (
	ProtoVersion  = 1
	MinNameLen    = 1
	MaxNameLen    = 24
	MaxChatMsgLen = 2 * 1024 // 2 KB
)

var nameRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// ValidateName returns a non-nil error if name violates the naming rules.
func ValidateName(name string) error {
	if len(name) < MinNameLen || len(name) > MaxNameLen {
		return fmt.Errorf("name must be %d–%d characters", MinNameLen, MaxNameLen)
	}
	if !nameRe.MatchString(name) {
		return fmt.Errorf("name may only contain A-Z a-z 0-9 _ -")
	}
	return nil
}
