package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/CoolBanHub/aggo/knowledge"
	"github.com/CoolBanHub/aggo/tools"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent"
	"github.com/cloudwego/eino/flow/agent/multiagent/host"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
)

type KnowledgeAgent struct {
	name        string
	description string

	knowledgeManager *knowledge.KnowledgeManager

	// 知识库查询配置
	knowledgeConfig *KnowledgeQueryConfig

	agent *react.Agent
}

func NewKnowledgeAgent(ctx context.Context, cm model.ToolCallingChatModel, knowledgeManager *knowledge.KnowledgeManager) (*KnowledgeAgent, error) {
	this := &KnowledgeAgent{
		knowledgeManager: knowledgeManager,
		name:             "knowledge_reason",
	}

	description := `You have access to the Think, Search, and Analyze tools that will help you search your knowledge for relevant information. Use these tools as frequently as needed to find the most relevant information.

        ## How to use the Think, Search, and Analyze tools:
        1. **Think**
        - Purpose: A scratchpad for planning, brainstorming keywords, and refining your approach. You never reveal your "Think" content to the user.
        - Usage: Call "think" whenever you need to figure out what to do next, analyze your approach, or decide new search terms before (or after) you look up documents.

        2. **Search**
        - Purpose: Executes a query against the knowledge base.
        - Usage: Call "search" with a clear query string whenever you want to retrieve documents or data. You can and should call this tool multiple times in one conversation.
            - For complex topics, use multiple focused searches rather than one broad search
            - Try different phrasing and keywords if initial searches don't yield useful results
            - Use quotes for exact phrases and OR for alternative terms (e.g., "protein synthesis" OR "protein formation")

        3. **Analyze**
        - Purpose: Evaluate whether the returned documents are correct and sufficient. If not, go back to "Think" or "Search" with refined queries.
        - Usage: Call "analyze" after getting search results to verify the quality and correctness of that information. Consider:
            - Relevance: Do the documents directly address the user's question?
            - Completeness: Is there enough information to provide a thorough answer?
            - Reliability: Are the sources credible and up-to-date?
            - Consistency: Do the documents agree or contradict each other?

        **Important Guidelines**:
        - Do not include your internal chain-of-thought in direct user responses.
        - Use "Think" to reason internally. These notes are never exposed to the user.
        - Iterate through the cycle (Think → Search → Analyze) as many times as needed until you have a final answer.
        - When you do provide a final answer to the user, be clear, concise, and accurate.
        - If search results are sparse or contradictory, acknowledge limitations in your response.
        - Synthesize information from multiple sources rather than relying on a single document.`
	this.description = description

	reactAgent, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: cm,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: tools.GetKnowledgeReasoningTools(this.knowledgeManager),
		},
		ToolReturnDirectly: map[string]struct{}{},
		MessageModifier: func(ctx context.Context, input []*schema.Message) []*schema.Message {
			return input
		},
	})
	if err != nil {
		return nil, err
	}

	this.agent = reactAgent

	return this, nil
}

func (this *KnowledgeAgent) Generate(ctx context.Context, input []*schema.Message) (*schema.Message, error) {
	// 直接使用传入的 input 调用内部 agent
	return this.agent.Generate(ctx, input)
}

func (this *KnowledgeAgent) Stream(ctx context.Context, input []*schema.Message) (*schema.StreamReader[*schema.Message], error) {
	return this.agent.Stream(ctx, input)
}

func (this *KnowledgeAgent) NewSpecialist() *host.Specialist {

	return &host.Specialist{
		AgentMeta: host.AgentMeta{
			Name:        this.name,
			IntendedUse: this.description,
		},
		Invokable: func(ctx context.Context, input []*schema.Message, opts ...agent.AgentOption) (output *schema.Message, err error) {
			return this.Generate(ctx, input)
		},
		Streamable: func(ctx context.Context, input []*schema.Message, opts ...agent.AgentOption) (output *schema.StreamReader[*schema.Message], err error) {
			return this.Stream(ctx, input)
		},
	}
}

// Run 实现工具调用接口，将消息转换为字符串响应
func (this *KnowledgeAgent) Run(ctx context.Context, param any) (string, error) {
	input := ctx.Value("messages").([]*schema.Message)
	j, _ := json.Marshal(input)
	fmt.Println("ctxmessage:", string(j))
	r, err := this.Generate(ctx, input)
	if err != nil {
		return "", err
	}
	return r.Content, nil
}
func (this *KnowledgeAgent) createTool() tool.InvokableTool {
	addUserTool := utils.NewTool(&schema.ToolInfo{
		Name: this.name,
		Desc: this.description,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"reason": {
				Type: schema.String,
				Desc: "the reason to call this tool",
			},
		}),
	}, this.Run)
	return addUserTool
}
