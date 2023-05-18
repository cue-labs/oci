package ociregistry

type Iter[T any] interface {
	Close()
	Next() (T, bool)
	Error() error
}

func All[T any](it Iter[T]) ([]T, error) {
	var xs []T
	for {
		x, ok := it.Next()
		if !ok {
			return xs, it.Error()
		}
		xs = append(xs, x)
	}
}

type sliceIter[T any] struct {
	i  int
	xs []T
}

func SliceIter[T any](xs []T) Iter[T] {
	return &sliceIter[T]{
		xs: xs,
	}
}

func (it *sliceIter[T]) Close() {}

func (it *sliceIter[T]) Next() (T, bool) {
	if it.i >= len(it.xs) {
		return *new(T), false
	}
	x := it.xs[it.i]
	it.i++
	return x, true
}

func (it *sliceIter[T]) Error() error {
	return nil
}
