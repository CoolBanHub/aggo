package glm

import (
	"github.com/cloudwego/eino/components/model"
)

// options is the specific options for the glm
type options struct {
	// Thinking enables thinking mode
	// Optional. Default: base on the Model
	Thinking *string
}

// WithThinking is the option to set the enable thinking for the model.
func WithThinking(thinking string) model.Option {
	return model.WrapImplSpecificOptFn(func(opt *options) {
		opt.Thinking = &thinking
	})
}
