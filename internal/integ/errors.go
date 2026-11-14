package integ

import "fmt"

// StatusError is a non-200 answer of a service.
type StatusError struct {
	Service string
	Status  int
}

func (e *StatusError) Error() string { return fmt.Sprintf("%s answered HTTP %d", e.Service, e.Status) }
