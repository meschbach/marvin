package junk

import "fmt"

// OperationalError provides human readable context to an error
type OperationalError struct {
	Description string
	Underlying  error
}

func (o *OperationalError) Error() string {
	return fmt.Sprintf("%s: %s", o.Description, o.Underlying.Error())
}

func (o *OperationalError) Unwrap() error { return o.Underlying }
