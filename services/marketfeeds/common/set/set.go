package set

type Set[T comparable] map[T]struct{}

func FromSlice[T comparable](items []T) Set[T] {
	result := make(Set[T])
	for _, item := range items {
		result[item] = struct{}{}
	}
	return result
}

func (s Set[T]) Include(value T) bool {
	_, found := s[value]
	return found
}

func (s Set[T]) Insert(value T) {
	s[value] = struct{}{}
}

func (s Set[T]) Delete(value T) {
	delete(s, value)
}

func (s Set[T]) Slice() []T {
	result := make([]T, len(s))
	i := 0
	for item := range s {
		result[i] = item
		i++
	}
	return result
}
