/*
 * Copyright 2024 CloudWeGo Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package glm

import (
	"context"
	"testing"

	"github.com/CoolBanHub/aggo/utils"
	"github.com/cloudwego/eino/schema"
)

func TestChatModel(t *testing.T) {
	ctx := context.Background()
	cm, err := NewChatModel(ctx, &ChatModelConfig{
		APIKey:   "api-key",
		Model:    "glm-4.5-flash",
		Thinking: utils.ValueToPtr(ThinkingEnabled),
	})
	if err != nil {
		t.Error(err)
		return
	}
	msg, err := cm.Generate(ctx, []*schema.Message{
		schema.UserMessage("hello"),
	})
	if err != nil {
		t.Errorf("Generate failed: %v", err)
		return
	}
	t.Logf("Thinking result: %s", msg.ReasoningContent)
	t.Logf("Generate result: %s", msg.Content)
}
