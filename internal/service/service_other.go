//go:build !windows

package service

import (
	"context"
)

func Host(func(context.Context, func()) error) (bool, error) { return false, nil }
