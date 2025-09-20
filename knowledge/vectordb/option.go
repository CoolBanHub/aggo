package vectordb

type Option struct {
	TopK    int
	filters map[string]any
}
