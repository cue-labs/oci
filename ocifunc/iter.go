package ocifunc

import "github.com/rogpeppe/ociregistry"

// ErrIter implements ociregistry.Iter by returning
// no items and the value of the Err field from the Error
// method.
type ErrIter[T any] struct {
	Err error
}

var _ ociregistry.Iter[int] = ErrIter[int]{}

func (it ErrIter[T]) Close() {}

func (it ErrIter[T]) Next() (T, bool) {
	return *new(T), false
}

func (it ErrIter[T]) Error() error {
	return it.Err
}
