//go:build !linux

package packages

import "context"

func Collect(context.Context) ([]Package, error) {
	return nil, ErrUnsupported
}
