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
