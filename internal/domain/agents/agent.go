package agents

import "errors"

var ErrNotFound = errors.New("agent not found")

type Agent struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Category          string   `json:"category"`
	Tags              []string `json:"tags"`
	Description       string   `json:"description"`
	PriceUses         int      `json:"priceUses"`
	PriceCents        int      `json:"priceCents"`
	Rating            string   `json:"rating"`
	DealCount         int      `json:"dealCount"`
	EstimatedDuration string   `json:"estimatedDuration"`
	Deliverable       string   `json:"deliverable"`
	ArtifactTypes     []string `json:"artifactTypes"`

	// Runtime selects the local agent CLI that actually executes this agent's
	// tasks ("codex" or "claude"). It is what turns a catalog entry into a real
	// long-horizon worker rather than a placeholder.
	Runtime string `json:"runtime"`
	// Sandbox is the permission level the runtime runs under
	// ("read-only", "workspace-write", "full"). Empty means workspace-write.
	Sandbox string `json:"sandbox,omitempty"`
	// Model optionally overrides the runtime's default model.
	Model string `json:"model,omitempty"`
	// SystemPrompt is the persona/role instruction prepended to the user's task
	// when invoking the runtime. It is internal prompt engineering and is not
	// exposed over the user-facing API.
	SystemPrompt string `json:"-"`
}

// Runtime identifiers understood by the execution engine. Kept as plain strings
// here to avoid a domain dependency on the agentrun package.
const (
	RuntimeCodex  = "codex"
	RuntimeClaude = "claude"
)

func SeedCatalog() []Agent {
	return []Agent{
		{
			ID:                "research-analyst",
			Name:              "行业研究分析师",
			Category:          "research",
			Tags:              []string{"专家", "竞品分析", "趋势洞察"},
			Description:       "深度行业研究与竞品洞察，输出结构化分析报告。",
			PriceUses:         1,
			PriceCents:        1990,
			Rating:            "4.9",
			DealCount:         1268,
			EstimatedDuration: "2-4 小时",
			Deliverable:       "市场规模、竞争格局、趋势洞察、机会清单",
			ArtifactTypes:     []string{"Markdown 报告"},
			Runtime:           RuntimeCodex,
			Sandbox:           "workspace-write",
			SystemPrompt: "你是一名资深行业研究分析师。基于用户给出的主题，产出一份结构化的中文研究报告，" +
				"涵盖市场规模、竞争格局、趋势洞察与机会清单。把报告写入当前工作目录下的 report.md 文件，使用清晰的 Markdown 标题与表格。",
		},
		{
			ID:                "content-growth",
			Name:              "内容增长写手",
			Category:          "content",
			Tags:              []string{"热门", "内容运营", "短视频脚本"},
			Description:       "生成公众号、朋友圈、短视频脚本与产品文案。",
			PriceUses:         1,
			PriceCents:        880,
			Rating:            "4.8",
			DealCount:         3420,
			EstimatedDuration: "30-60 分钟",
			Deliverable:       "标题方向、正文、分发建议",
			ArtifactTypes:     []string{"Markdown 文案"},
			Runtime:           RuntimeClaude,
			Sandbox:           "workspace-write",
			SystemPrompt: "你是一名擅长内容增长的写手。根据用户的需求产出可直接发布的中文内容，" +
				"包含标题方向、正文与分发建议。把成稿写入当前工作目录下的 content.md。",
		},
		{
			ID:                "business-data",
			Name:              "经营数据分析师",
			Category:          "data",
			Tags:              []string{"企业", "指标拆解", "异常诊断"},
			Description:       "处理经营数据，定位异常波动并输出业务建议。",
			PriceUses:         1,
			PriceCents:        1560,
			Rating:            "4.7",
			DealCount:         986,
			EstimatedDuration: "1-2 小时",
			Deliverable:       "指标拆解、异常解释、改进建议",
			ArtifactTypes:     []string{"Markdown 报告", "CSV 摘要"},
			Runtime:           RuntimeCodex,
			Sandbox:           "workspace-write",
			SystemPrompt: "你是一名经营数据分析师。根据用户描述的数据与问题，做指标拆解、异常解释并给出改进建议。" +
				"把分析写入当前工作目录下的 analysis.md；如果生成了汇总数据，另存为 summary.csv。",
		},
		{
			ID:                "launch-planner",
			Name:              "新品上市策划",
			Category:          "marketing",
			Tags:              []string{"增长", "营销节奏", "渠道策略"},
			Description:       "为新品制定目标客群、渠道节奏与首轮推广方案。",
			PriceUses:         1,
			PriceCents:        1290,
			Rating:            "4.8",
			DealCount:         2104,
			EstimatedDuration: "1 小时",
			Deliverable:       "人群画像、卖点主张、投放节奏",
			ArtifactTypes:     []string{"Markdown 报告"},
			Runtime:           RuntimeClaude,
			Sandbox:           "workspace-write",
			SystemPrompt: "你是一名新品上市策划。根据用户的产品信息，输出目标人群画像、核心卖点主张与首轮投放节奏。" +
				"把方案写入当前工作目录下的 launch-plan.md。",
		},
	}
}
