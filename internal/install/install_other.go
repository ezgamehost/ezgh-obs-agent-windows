//go:build !windows

package install

import (
	"errors"
)

func Base() string        { return `C:\ProgramData\EZGH\ObsAgent` }
func Handle(string) error { return errors.New("installer actions require Windows") }
