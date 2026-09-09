package common

func Pointer[T any](t T) *T {
	return &t
}

