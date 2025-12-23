package query

import "fmt"

// operationalError provides human readable context to an error
type operationalError struct {
	description string
	underlying  error
}

func (o *operationalError) Error() string {
	return fmt.Sprintf("%s: %s", o.description, o.underlying.Error())
}

func (o *operationalError) Unwrap() error { return o.underlying }
