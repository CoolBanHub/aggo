package postgres

type Option struct {
	TopK    int
	Filters map[string]any
}
