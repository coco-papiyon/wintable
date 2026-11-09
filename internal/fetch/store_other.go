//go:build !windows

package fetch

import (
	"context"
	"fmt"
)

type Store struct{}

func Preflight(string) error { return fmt.Errorf("Windows controller required") }
func Begin(context.Context, string) (*Store, error) {
	return nil, fmt.Errorf("Windows controller required")
}
func (*Store) Write([]byte) (int, error) { return 0, fmt.Errorf("Windows controller required") }
func (*Store) Commit(context.Context, [32]byte) (bool, error) {
	return false, fmt.Errorf("Windows controller required")
}
func (*Store) Close() error { return nil }
