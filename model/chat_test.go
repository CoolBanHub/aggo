package model

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// 必填项缺失时在构造阶段返回清晰错误，不发网络请求。
func TestNewChatModelValidatesRequiredFields(t *testing.T) {
	cases := []struct {
		name string
		opts []OptionFunc
		want string
	}{
		{"missing model", []OptionFunc{WithAPIKey("k")}, "WithModel"},
		{"missing api key", []OptionFunc{WithModel("gpt-x")}, "WithAPIKey"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewChatModelContext(context.Background(), tc.opts...)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want mention of %s", err, tc.want)
			}
		})
	}
}

// WithBaseURL 与 WithBaseUrl 是同一配置的两个入口。
func TestWithBaseURLAlias(t *testing.T) {
	o := &Option{}
	WithBaseURL("https://a.example.com")(o)
	if o.BaseUrl != "https://a.example.com" {
		t.Fatalf("WithBaseURL = %q", o.BaseUrl)
	}
	WithBaseUrl("https://b.example.com")(o)
	if o.BaseUrl != "https://b.example.com" {
		t.Fatalf("WithBaseUrl = %q", o.BaseUrl)
	}
}

// 新增的 HTTP/Timing/Header 选项写入 Option 并能被读取。
func TestExtendedOptions(t *testing.T) {
	o := &Option{}
	client := &http.Client{Timeout: 5 * time.Second}
	WithHTTPClient(client)(o)
	WithTimeout(30 * time.Second)(o)
	WithCustomHeaders(map[string]string{"X-Trace": "1"})(o)
	WithExtraFields(map[string]any{"user": "u1"})(o)

	if o.HTTPClient != client {
		t.Fatal("HTTPClient not set")
	}
	if o.Timeout != 30*time.Second {
		t.Fatalf("Timeout = %v", o.Timeout)
	}
	if o.CustomHeaders["X-Trace"] != "1" {
		t.Fatalf("CustomHeaders = %v", o.CustomHeaders)
	}
	if o.ExtraFields["user"] != "u1" {
		t.Fatalf("ExtraFields = %v", o.ExtraFields)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
