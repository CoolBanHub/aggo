package utils

type ToPtr interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~uintptr |
		~float32 | ~float64 |
		~string |
		~bool
}

func ValueToPtr[T ToPtr](f T) *T {
	return &f
}
