package ghcr

import "errors"

const keepCountInvalidErrorMessageConstant = "keep count must be greater than zero"

// KeepCount is a validated number of package versions to retain.
type KeepCount struct {
	value int
}

// NewKeepCount validates and constructs a package-version retention count.
func NewKeepCount(value int) (KeepCount, error) {
	if value <= 0 {
		return KeepCount{}, errors.New(keepCountInvalidErrorMessageConstant)
	}

	return KeepCount{value: value}, nil
}

// Value returns the validated retention count.
func (count KeepCount) Value() int {
	return count.value
}
