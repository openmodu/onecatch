import { useEffect, useMemo, useState } from "react";
import {
  AgentBinding,
  ArtifactBinding,
  AuthBinding,
  BillingBinding,
  OrderBinding,
} from "../../bindings/github.com/openmodu/oneshot/desktop/oneshot/bindings/index.js";

const navigation = [
  { id: "workbench", label: "工作台", icon: "W" },
  { id: "orders", label: "我的订单", icon: "O" },
  { id: "billing", label: "用量账单", icon: "U" },
  { id: "account", label: "账户", icon: "A" },
];

const categories = [
  { id: "all", label: "全部 Agent" },
  { id: "content", label: "内容创作" },
  { id: "data", label: "数据分析" },
  { id: "marketing", label: "市场营销" },
  { id: "design", label: "设计创意" },
  { id: "dev", label: "开发与技术" },
  { id: "office", label: "办公效率" },
  { id: "research", label: "行业研究" },
];

const orderFilters = [
  { id: "", label: "全部订单" },
  { id: "pending_payment", label: "待支付" },
  { id: "running", label: "执行中" },
  { id: "delivered", label: "已交付" },
];

const workflowSteps = [
  { id: "details", label: "Agent 详情" },
  { id: "requirement", label: "需求填写" },
  { id: "checkout", label: "扣次确认" },
  { id: "running", label: "执行中" },
  { id: "artifact", label: "交付物" },
];

const inspectorPanels = [
  { id: "preview", label: "预览", icon: "P", shortcut: "⌘P" },
  { id: "detail", label: "明细", icon: "D", shortcut: "⌘D" },
  { id: "order", label: "订单", icon: "O", shortcut: "⌘O" },
  { id: "records", label: "记录", icon: "R", shortcut: "⌘R" },
  { id: "progress", label: "进度", icon: "S", shortcut: "⌘J" },
];

const fallbackAgents = [
  {
    id: "research-analyst",
    name: "行业研究分析师",
    category: "research",
    tags: ["专家", "竞品分析", "趋势洞察"],
    description: "深度行业研究与竞品洞察，输出结构化分析报告。",
    priceUses: 1,
    priceCents: 1990,
    rating: "4.9",
    dealCount: 1268,
    estimatedDuration: "2-4 小时",
    deliverable: "市场规模、竞争格局、趋势洞察、机会清单",
    artifactTypes: ["PDF 报告"],
  },
  {
    id: "content-growth",
    name: "内容增长写手",
    category: "content",
    tags: ["热门", "内容运营", "短视频脚本"],
    description: "生成公众号、朋友圈、短视频脚本与产品文案。",
    priceUses: 1,
    priceCents: 880,
    rating: "4.8",
    dealCount: 3420,
    estimatedDuration: "30-60 分钟",
    deliverable: "标题方向、正文、分发建议",
    artifactTypes: ["PDF 报告", "Markdown 文案"],
  },
  {
    id: "business-data",
    name: "经营数据分析师",
    category: "data",
    tags: ["企业", "指标拆解", "异常诊断"],
    description: "处理经营数据，定位异常波动并输出业务建议。",
    priceUses: 1,
    priceCents: 1560,
    rating: "4.7",
    dealCount: 986,
    estimatedDuration: "1-2 小时",
    deliverable: "指标拆解、异常解释、改进建议",
    artifactTypes: ["PDF 报告", "CSV 摘要"],
  },
  {
    id: "launch-planner",
    name: "新品上市策划",
    category: "marketing",
    tags: ["增长", "营销节奏", "渠道策略"],
    description: "为新品制定目标客群、渠道节奏与首轮推广方案。",
    priceUses: 1,
    priceCents: 1290,
    rating: "4.8",
    dealCount: 2104,
    estimatedDuration: "1 小时",
    deliverable: "人群画像、卖点主张、投放节奏",
    artifactTypes: ["PDF 报告"],
  },
];

const fallbackOrders = [
  {
    id: "ORD20260610001029",
    agentName: "行业研究分析师",
    requirement: {
      prompt:
        "请帮我完成 2026 年中国 AI Agent 服务市场研究，包括市场规模、主要玩家、收费模式、增长机会和进入建议。",
    },
    status: "running",
    usageCost: 1,
    amountCents: 1990,
    estimatedCompletionAt: "2026-06-10T12:30:00+08:00",
    createdAt: "2026-06-10T10:30:00+08:00",
    progress: [
      { key: "submitted", label: "需求已提交", state: "done", timestamp: "2026-06-10T10:30:00+08:00" },
      { key: "checkout", label: "扣次确认", state: "done", timestamp: "2026-06-10T10:31:00+08:00" },
      { key: "running", label: "执行中", state: "current", timestamp: "2026-06-10T10:32:00+08:00" },
      { key: "delivering", label: "交付物生成中", state: "next" },
      { key: "delivered", label: "已交付", state: "next" },
    ],
  },
  {
    id: "ORD20260609000988",
    agentName: "经营数据分析师",
    requirement: { prompt: "竞品分析报告" },
    status: "delivered",
    usageCost: 1,
    amountCents: 1560,
    estimatedCompletionAt: "2026-06-09T17:36:00+08:00",
    createdAt: "2026-06-09T16:24:00+08:00",
    progress: [],
  },
];

const fallbackArtifacts = [
  {
    id: "artifact-demo",
    fileName: "2026年中国AI Agent服务市场研究报告.pdf",
    fileType: "PDF",
    sizeBytes: 2457600,
    preview: "市场规模、竞争格局、趋势洞察、机会清单",
  },
];

function canUseFallback() {
  return import.meta.env.DEV && typeof window !== "undefined" && !window.__ONESOT_DISABLE_FIXTURE__;
}

function formatMoney(cents) {
  return `¥ ${(Number(cents || 0) / 100).toFixed(2)}`;
}

function formatDealCount(value) {
  return Number(value || 0).toLocaleString("zh-CN");
}

function formatDateTime(value) {
  if (!value) return "待确认";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "待确认";
  return date.toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function statusLabel(status) {
  const labels = {
    draft: "草稿",
    pending_payment: "待支付",
    paid: "已支付",
    running: "执行中",
    delivering: "交付生成中",
    delivered: "已交付",
    failed: "失败",
    cancelled: "已取消",
  };
  return labels[status] || "待确认";
}

function ledgerLabel(type) {
  const labels = {
    purchase: "购买次数",
    debit: "执行扣次",
    refund: "退回次数",
  };
  return labels[type] || "用量变动";
}

function SegmentControl({ items, value, onChange, label }) {
  return (
    <div className="segmented" aria-label={label}>
      {items.map((item) => (
        <button
          className={value === item.id ? "selected" : ""}
          key={item.id}
          type="button"
          onClick={() => onChange(item.id)}
        >
          {item.label}
        </button>
      ))}
    </div>
  );
}

export default function App() {
  const [agents, setAgents] = useState([]);
  const [balance, setBalance] = useState(null);
  const [ledger, setLedger] = useState([]);
  const [userOrders, setUserOrders] = useState([]);
  const [artifacts, setArtifacts] = useState([]);
  const [currentUser, setCurrentUser] = useState(null);
  const [authProvider, setAuthProvider] = useState("");
  const [authStatus, setAuthStatus] = useState("checking");
  const [agentStatus, setAgentStatus] = useState("loading");
  const [toast, setToast] = useState("");
  const [activeView, setActiveView] = useState("workbench");
  const [activeCategory, setActiveCategory] = useState("all");
  const [selectedAgentId, setSelectedAgentId] = useState("");
  const [selectedStep, setSelectedStep] = useState("checkout");
  const [selectedOrderId, setSelectedOrderId] = useState("");
  const [orderFilter, setOrderFilter] = useState("");
  const [inspectorMenuOpen, setInspectorMenuOpen] = useState(false);
  const [activeInspectorPanel, setActiveInspectorPanel] = useState("detail");
  const [inspectorPanelOpen, setInspectorPanelOpen] = useState(false);
  const [requirement, setRequirement] = useState(
    "请帮我完成 2026 年中国 AI Agent 服务市场研究，包括市场规模、主要玩家、收费模式、增长机会和进入建议。",
  );

  const visibleAgents = useMemo(() => {
    if (activeCategory === "all") return agents;
    return agents.filter((agent) => agent.category === activeCategory);
  }, [activeCategory, agents]);

  const selectedAgent =
    agents.find((agent) => agent.id === selectedAgentId) ?? visibleAgents[0] ?? null;
  const selectedOrder =
    userOrders.find((order) => order.id === selectedOrderId) ?? userOrders[0] ?? null;
  const currentBalance = balance?.remaining ?? 0;
  const authProviderLabel = authProvider === "google" ? "Google 邮箱" : "微信";
  const recentLedger = ledger.slice(-5).reverse();
  const inspectorArtifacts =
    artifacts.length > 0 || selectedOrder?.status !== "delivered" ? artifacts : fallbackArtifacts;
  const timeline =
    selectedOrder?.progress?.length > 0
      ? selectedOrder.progress
      : [
          { key: "submitted", label: "需求已提交", state: selectedOrder ? "done" : "next" },
          { key: "checkout", label: "扣次确认", state: selectedOrder ? "done" : "current" },
          { key: "running", label: "执行中", state: selectedOrder?.status === "running" ? "current" : "next" },
          { key: "delivering", label: "交付物生成中", state: selectedOrder?.status === "delivering" ? "current" : "next" },
          { key: "delivered", label: "已交付", state: selectedOrder?.status === "delivered" ? "done" : "next" },
        ];

  useEffect(() => {
    let alive = true;
    AgentBinding.ListAgents()
      .then((items) => {
        if (!alive) return;
        const catalog = Array.isArray(items) ? items : [];
        setAgents(catalog);
        setSelectedAgentId((current) =>
          catalog.some((agent) => agent.id === current) ? current : catalog[0]?.id || "",
        );
        setAgentStatus("ready");
      })
      .catch((error) => {
        if (!alive) return;
        if (canUseFallback()) {
          setAgents(fallbackAgents);
          setSelectedAgentId((current) =>
            fallbackAgents.some((agent) => agent.id === current) ? current : fallbackAgents[0].id,
          );
          setAgentStatus("fixture");
          return;
        }
        setAgents([]);
        setSelectedAgentId("");
        setAgentStatus("failed");
        showToast(error?.message || "Agent 目录加载失败");
      });
    return () => {
      alive = false;
    };
  }, []);

  useEffect(() => {
    let alive = true;
    AuthBinding.CurrentUser()
      .then((user) => {
        if (!alive) return;
        if (user?.id) {
          setCurrentUser(user);
          setAuthProvider(window.localStorage.getItem("oneshot.authProvider") || "wechat");
          setAuthStatus("signed-in");
          return;
        }
        setCurrentUser(null);
        setAuthProvider("");
        setAuthStatus("signed-out");
      })
      .catch(() => {
        if (!alive) return;
        setCurrentUser(null);
        setAuthProvider("");
        setAuthStatus("signed-out");
        if (canUseFallback()) {
          setBalance({ remaining: 12 });
          setLedger([
            { id: "ledger-demo-1", type: "purchase", delta: 10, balanceAfter: 12, createdAt: "2026-06-10T09:00:00+08:00" },
            { id: "ledger-demo-2", type: "debit", delta: -1, balanceAfter: 11, createdAt: "2026-06-10T10:31:00+08:00" },
          ]);
          setUserOrders(fallbackOrders);
          setSelectedOrderId(fallbackOrders[0].id);
        }
      });
    return () => {
      alive = false;
    };
  }, []);

  useEffect(() => {
    if (!currentUser?.id) {
      if (!canUseFallback()) {
        setBalance(null);
        setLedger([]);
        setUserOrders([]);
        setSelectedOrderId("");
        setArtifacts([]);
      }
      return undefined;
    }
    refreshBilling();
    refreshOrders(orderFilter);
    const timer = window.setInterval(() => refreshOrders(orderFilter), 2000);
    return () => window.clearInterval(timer);
  }, [currentUser?.id, orderFilter]);

  useEffect(() => {
    let alive = true;
    if (!selectedOrder?.id || selectedOrder.status !== "delivered") {
      setArtifacts([]);
      return undefined;
    }
    ArtifactBinding.ListArtifacts(selectedOrder.id)
      .then((items) => {
        if (!alive) return;
        setArtifacts(Array.isArray(items) ? items : []);
      })
      .catch(() => {
        if (!alive) return;
        setArtifacts(canUseFallback() ? fallbackArtifacts : []);
      });
    return () => {
      alive = false;
    };
  }, [selectedOrder?.id, selectedOrder?.status]);

  function showToast(message) {
    setToast(message);
    window.clearTimeout(showToast.timer);
    showToast.timer = window.setTimeout(() => setToast(""), 2200);
  }

  async function refreshBilling() {
    try {
      const [nextBalance, nextLedger] = await Promise.all([
        BillingBinding.GetBalance(),
        BillingBinding.ListLedger(),
      ]);
      setBalance(nextBalance);
      setLedger(Array.isArray(nextLedger) ? nextLedger : []);
    } catch (error) {
      if (canUseFallback()) return;
      showToast(error?.message || "用量信息加载失败");
    }
  }

  async function refreshOrders(status = orderFilter) {
    try {
      const items = await OrderBinding.ListOrders(status || "");
      const nextOrders = Array.isArray(items) ? items : [];
      setUserOrders(nextOrders);
      setSelectedOrderId((current) =>
        nextOrders.some((order) => order.id === current) ? current : nextOrders[0]?.id || "",
      );
    } catch (error) {
      if (canUseFallback()) return;
      showToast(error?.message || "订单列表加载失败");
    }
  }

  async function login(provider) {
    setAuthStatus("checking");
    try {
      const session =
        provider === "google"
          ? await AuthBinding.LoginWithGoogle()
          : await AuthBinding.LoginWithWechat();
      setCurrentUser(session.user);
      setAuthProvider(session.provider || provider);
      window.localStorage.setItem("oneshot.authProvider", session.provider || provider);
      setAuthStatus("signed-in");
      await Promise.all([refreshBilling(), refreshOrders(orderFilter)]);
      showToast(`已通过${provider === "google" ? " Google 邮箱" : "微信"}登录`);
    } catch (error) {
      if (canUseFallback()) {
        const fixtureUser = {
          id: "local-preview-user",
          displayName: provider === "google" ? "Google 邮箱用户" : "微信用户",
          email: provider === "google" ? "preview@oneshot.local" : "",
        };
        setCurrentUser(fixtureUser);
        setAuthProvider(provider);
        window.localStorage.setItem("oneshot.authProvider", provider);
        setAuthStatus("signed-in");
        setBalance({ remaining: 12 });
        setUserOrders(fallbackOrders);
        setSelectedOrderId(fallbackOrders[0].id);
        showToast("已进入本地预览登录态");
        return;
      }
      setAuthStatus("signed-out");
      showToast(error?.message || "登录失败，请稍后重试");
    }
  }

  async function logout() {
    try {
      await AuthBinding.Logout();
      setCurrentUser(null);
      setAuthProvider("");
      window.localStorage.removeItem("oneshot.authProvider");
      setAuthStatus("signed-out");
      showToast("已退出登录");
    } catch (error) {
      if (canUseFallback()) {
        setCurrentUser(null);
        setAuthProvider("");
        window.localStorage.removeItem("oneshot.authProvider");
        setAuthStatus("signed-out");
        showToast("已退出本地预览登录态");
        return;
      }
      showToast(error?.message || "退出登录失败");
    }
  }

  async function buyUses() {
    if (!currentUser && !canUseFallback()) {
      showToast("请先登录后再购买次数");
      setActiveView("account");
      return;
    }
    try {
      await BillingBinding.StartPurchase({ planId: "uses_10" });
      await refreshBilling();
      showToast("已购买 10 次");
    } catch (error) {
      if (canUseFallback()) {
        setBalance((value) => ({ remaining: (value?.remaining ?? 12) + 10 }));
        setLedger((items) => [
          ...items,
          {
            id: `ledger-${Date.now()}`,
            type: "purchase",
            delta: 10,
            balanceAfter: (balance?.remaining ?? 12) + 10,
            createdAt: new Date().toISOString(),
          },
        ]);
        showToast("本地预览已增加 10 次");
        return;
      }
      showToast(error?.message || "购买次数失败");
    }
  }

  async function confirmCheckout() {
    if (!selectedAgent) {
      showToast("请先选择可用 Agent");
      return;
    }
    if (!currentUser && !canUseFallback()) {
      showToast("请先登录后再创建订单");
      setActiveView("account");
      return;
    }
    if (!requirement.trim()) {
      showToast("请先填写任务需求");
      setSelectedStep("requirement");
      return;
    }
    try {
      const order = await OrderBinding.CreateOrder({
        agentId: selectedAgent.id,
        requirement: { prompt: requirement },
      });
      setSelectedOrderId(order.id);
      setSelectedStep("running");
      await Promise.all([refreshBilling(), refreshOrders("")]);
      showToast("已开始执行");
    } catch (error) {
      if (canUseFallback()) {
        const order = {
          id: `ORD${Date.now().toString().slice(-12)}`,
          agentName: selectedAgent.name,
          requirement: { prompt: requirement },
          status: "running",
          usageCost: selectedAgent.priceUses || 1,
          amountCents: selectedAgent.priceCents,
          estimatedCompletionAt: new Date(Date.now() + 90 * 60 * 1000).toISOString(),
          createdAt: new Date().toISOString(),
          progress: [
            { key: "submitted", label: "需求已提交", state: "done", timestamp: new Date().toISOString() },
            { key: "checkout", label: "扣次确认", state: "done", timestamp: new Date().toISOString() },
            { key: "running", label: "执行中", state: "current", timestamp: new Date().toISOString() },
            { key: "delivering", label: "交付物生成中", state: "next" },
            { key: "delivered", label: "已交付", state: "next" },
          ],
        };
        setUserOrders((items) => [order, ...items]);
        setSelectedOrderId(order.id);
        setSelectedStep("running");
        setBalance((value) => ({ remaining: Math.max(0, (value?.remaining ?? 12) - 1) }));
        showToast("本地预览已开始执行");
        return;
      }
      showToast(error?.message || "创建订单失败");
    }
  }

  async function cancelSelectedOrder() {
    if (!selectedOrder?.id) return;
    try {
      await OrderBinding.CancelOrder(selectedOrder.id);
      await refreshOrders(orderFilter);
      showToast("订单已取消");
    } catch (error) {
      showToast(error?.message || "取消失败");
    }
  }

  async function downloadArtifact(artifactId) {
    try {
      const download = await ArtifactBinding.DownloadArtifact(artifactId);
      if (download.filePath) {
        await ArtifactBinding.ShowInFolder(download.filePath);
      }
      showToast(download.filePath ? `已下载到 ${download.filePath}` : "已下载交付物");
    } catch (error) {
      if (canUseFallback()) {
        showToast("本地预览已模拟下载");
        return;
      }
      showToast(error?.message || "下载失败");
    }
  }

  async function shareArtifact(artifactId) {
    try {
      const share = await ArtifactBinding.ShareArtifact(artifactId);
      await navigator.clipboard?.writeText?.(share.url);
      showToast("分享链接已生成");
    } catch (error) {
      if (canUseFallback()) {
        showToast("本地预览已模拟分享");
        return;
      }
      showToast(error?.message || "分享失败");
    }
  }

  function chooseAgent(agent) {
    setSelectedAgentId(agent.id);
    setSelectedStep("details");
    setActiveView("workbench");
  }

  function chooseOrder(order) {
    setSelectedOrderId(order.id);
    setActiveView("orders");
  }

  function openInspectorPanel(panelId) {
    setActiveInspectorPanel(panelId);
    setInspectorPanelOpen(true);
    setInspectorMenuOpen(false);
  }

  function closeInspectorPanel() {
    setInspectorPanelOpen(false);
  }

  function renderInspectorMenu() {
    return (
      <div className="inspector-menu" role="menu" aria-label="Inspector 菜单">
        {inspectorPanels.map((panel) => (
          <button
            className={activeInspectorPanel === panel.id && inspectorPanelOpen ? "selected" : ""}
            key={panel.id}
            type="button"
            role="menuitem"
            onClick={() => openInspectorPanel(panel.id)}
          >
            <span className="menu-icon">{panel.icon}</span>
            <strong>{panel.label}</strong>
            <small>{panel.shortcut}</small>
          </button>
        ))}
      </div>
    );
  }

  function renderInspectorDetail(panelId) {
    if (panelId === "preview") {
      return (
        <div className="drawer-section">
          <div className="drawer-section-title">
            <span>交付物预览</span>
            <button type="button" onClick={() => setSelectedStep("artifact")}>定位</button>
          </div>
          {inspectorArtifacts[0] ? (
            <>
              <div className="file-row compact">
                <span className="file-icon">PDF</span>
                <div>
                  <strong>{inspectorArtifacts[0].fileName}</strong>
                  <small>{Math.max(1, Math.round((inspectorArtifacts[0].sizeBytes || 0) / 1024))} KB · {inspectorArtifacts[0].fileType}</small>
                </div>
              </div>
              <div className="report-preview drawer-preview">
                <strong>AI Agent 服务市场研究</strong>
                <span>市场规模 / 竞争格局 / 增长机会</span>
              </div>
              <div className="drawer-actions">
                <button type="button" onClick={() => shareArtifact(inspectorArtifacts[0].id)}>分享</button>
                <button type="button" onClick={() => downloadArtifact(inspectorArtifacts[0].id)}>下载</button>
              </div>
            </>
          ) : (
            <p className="muted-copy">订单交付后显示预览和下载入口。</p>
          )}
        </div>
      );
    }

    if (panelId === "order") {
      return (
        <div className="drawer-section">
          <div className="drawer-section-title">
            <span>订单信息</span>
            <button type="button" onClick={() => setActiveView("orders")}>打开订单</button>
          </div>
          {selectedOrder ? (
            <dl className="drawer-dl">
              <div><dt>订单号</dt><dd>{selectedOrder.id}</dd></div>
              <div><dt>关联 Agent</dt><dd>{selectedOrder.agentName}</dd></div>
              <div><dt>订单状态</dt><dd>{statusLabel(selectedOrder.status)}</dd></div>
              <div><dt>创建时间</dt><dd>{formatDateTime(selectedOrder.createdAt)}</dd></div>
              <div><dt>预计完成</dt><dd>{formatDateTime(selectedOrder.estimatedCompletionAt)}</dd></div>
              <div><dt>总金额</dt><dd>{formatMoney(selectedOrder.amountCents)}</dd></div>
            </dl>
          ) : (
            <p className="muted-copy">创建订单后会在这里显示订单信息。</p>
          )}
        </div>
      );
    }

    if (panelId === "records") {
      return (
        <div className="drawer-section">
          <div className="drawer-section-title">
            <span>使用记录</span>
            <button type="button" onClick={() => setActiveView("billing")}>查看全部</button>
          </div>
          <div className="drawer-list">
            {recentLedger.length > 0 ? recentLedger.map((entry) => (
              <div className="drawer-list-row" key={entry.id}>
                <span>
                  <strong>{ledgerLabel(entry.type)}</strong>
                  <small>{formatDateTime(entry.createdAt)}</small>
                </span>
                <em>{entry.delta > 0 ? `+${entry.delta}` : entry.delta} 次</em>
              </div>
            )) : (
              <p className="muted-copy">暂无用量记录。</p>
            )}
          </div>
        </div>
      );
    }

    if (panelId === "progress") {
      return (
        <div className="drawer-section">
          <div className="drawer-section-title">
            <span>执行进度</span>
            <button type="button" onClick={() => setSelectedStep("running")}>定位</button>
          </div>
          <div className="drawer-timeline">
            {timeline.map((step) => (
              <div className={`drawer-timeline-row ${step.state}`} key={step.key || step.label}>
                <span className="timeline-dot" />
                <strong>{step.label}</strong>
                <time>{step.timestamp ? formatDateTime(step.timestamp) : "待推进"}</time>
              </div>
            ))}
          </div>
        </div>
      );
    }

    return (
      <div className="drawer-section">
        <div className="drawer-section-title">
          <span>扣次与订单明细</span>
          <button type="button" onClick={buyUses}>购买次数</button>
        </div>
        <div className="detail-diff-list" aria-label="当前任务明细">
          <div className="detail-diff-row added">
            <span>余额</span>
            <strong>{currentBalance} 次</strong>
            <small>当前可用次数</small>
          </div>
          <div className="detail-diff-row removed">
            <span>本次扣减</span>
            <strong>-{selectedAgent?.priceUses || 1} 次</strong>
            <small>确认并支付后扣减</small>
          </div>
          <div className="detail-diff-row neutral">
            <span>单次价格</span>
            <strong>{selectedAgent ? formatMoney(selectedAgent.priceCents) : "待选择"}</strong>
            <small>{selectedAgent?.name || "请选择 Agent"}</small>
          </div>
          <div className="detail-diff-row neutral">
            <span>订单</span>
            <strong>{selectedOrder?.id || "待创建"}</strong>
            <small>{selectedOrder ? statusLabel(selectedOrder.status) : "当前任务尚未创建订单"}</small>
          </div>
        </div>
        <div className="drawer-section-title compact">
          <span>最近订单</span>
        </div>
        <div className="drawer-list">
          {userOrders.slice(0, 4).map((order) => (
            <button
              className={selectedOrder?.id === order.id ? "selected" : ""}
              key={order.id}
              type="button"
              onClick={() => chooseOrder(order)}
            >
              <span>
                <strong>{order.requirement?.prompt || "本次任务"}</strong>
                <small>{order.id}</small>
              </span>
              <em>{order.usageCost ? `-${order.usageCost} 次` : statusLabel(order.status)}</em>
            </button>
          ))}
          {userOrders.length === 0 && <p className="muted-copy">暂无订单。</p>}
        </div>
      </div>
    );
  }

  function renderInspectorPanel() {
    const panel =
      inspectorPanels.find((item) => item.id === activeInspectorPanel) ?? inspectorPanels[1];
    const contextName =
      activeView === "orders"
        ? "我的订单"
        : activeView === "billing"
          ? "用量账单"
          : activeView === "account"
            ? "账户"
            : selectedAgent?.name || "工作台";

    return (
      <aside className="inspector-drawer" aria-label={`${panel.label}面板`}>
        <header className="drawer-header">
          <div>
            <p>工作台 / {contextName}</p>
            <h2>{panel.label}</h2>
          </div>
          <button type="button" aria-label="关闭 Inspector 面板" onClick={closeInspectorPanel}>
            ×
          </button>
        </header>
        <div className="drawer-content">
          {renderInspectorDetail(panel.id)}
        </div>
      </aside>
    );
  }

  function renderWorkbench() {
    if (!selectedAgent) {
      return (
        <section className="empty-panel">
          {agentStatus === "loading" ? "正在加载 Agent 目录" : "该分类暂无可用 Agent"}
        </section>
      );
    }

    return (
      <section className="main-column">
        <section className="agent-hero">
          <div className="agent-mark" aria-hidden="true">
            {selectedAgent.name?.slice(0, 1) || "A"}
          </div>
          <div className="agent-copy">
            <div className="title-line">
              <h1>{selectedAgent.name}</h1>
              <span>{selectedAgent.tags?.[0] || "Agent"}</span>
            </div>
            <p>{selectedAgent.description}</p>
            <div className="meta-row">
              <strong>评分 {selectedAgent.rating || "-"}</strong>
              <span>成交 {formatDealCount(selectedAgent.dealCount)} 次</span>
              <span>平均交付 {selectedAgent.estimatedDuration || "待确认"}</span>
            </div>
          </div>
          <div className="price-box">
            <span>单次价格</span>
            <strong>{formatMoney(selectedAgent.priceCents)}</strong>
            <small>/ 次</small>
          </div>
        </section>

        <SegmentControl
          items={categories}
          label="Agent 分类"
          value={activeCategory}
          onChange={setActiveCategory}
        />

        <div className="agent-strip" aria-label="Agent 快捷切换">
          {visibleAgents.map((agent) => (
            <button
              className={selectedAgent.id === agent.id ? "selected" : ""}
              key={agent.id}
              type="button"
              onClick={() => chooseAgent(agent)}
            >
              <span>{agent.name.slice(0, 1)}</span>
              <strong>{agent.name}</strong>
              <em>{formatMoney(agent.priceCents)}</em>
            </button>
          ))}
        </div>

        <div className="flow-tabs" role="tablist" aria-label="任务流程">
          {workflowSteps.map((step, index) => (
            <button
              className={selectedStep === step.id ? "selected" : ""}
              key={step.id}
              type="button"
              onClick={() => setSelectedStep(step.id)}
            >
              <span>{index + 1}</span>
              {step.label}
            </button>
          ))}
        </div>

        <div className="brief-grid">
          <section className="panel requirement-panel">
            <div className="panel-title">
              <h2>{selectedStep === "requirement" ? "编辑需求" : "需求摘要"}</h2>
              <button type="button" onClick={() => setSelectedStep("requirement")}>
                编辑需求
              </button>
            </div>
            <textarea
              aria-label="任务需求"
              onChange={(event) => setRequirement(event.target.value)}
              readOnly={selectedStep !== "requirement"}
              value={requirement}
            />
          </section>

          <section className="panel checklist-panel">
            <h2>扣次说明</h2>
            <ul>
              <li>本次执行扣减 {selectedAgent.priceUses || 1} 次</li>
              <li>单次独立交付，仅针对本次需求</li>
              <li>开始执行后进入订单进度流转</li>
            </ul>
            <button className="soft-button" type="button" onClick={() => setSelectedStep("running")}>
              查看执行状态
            </button>
          </section>

          <section className="panel balance-panel">
            <h2>当前余额</h2>
            <div className="balance-number">
              <strong>{currentBalance}</strong>
              <span>次</span>
            </div>
            <button className="outline-button" type="button" onClick={buyUses}>
              购买次数
            </button>
          </section>
        </div>

        <section className="checkout-panel">
          <div className="calc-row">
            <span>本次扣减</span>
            <strong>{selectedAgent.priceUses || 1} 次</strong>
          </div>
          <div className="calc-row">
            <span>单次价格</span>
            <strong>{formatMoney(selectedAgent.priceCents)}</strong>
          </div>
          <div className="calc-row due">
            <span>应付金额</span>
            <strong>{formatMoney(selectedAgent.priceCents)}</strong>
          </div>
          <button className="primary-button" type="button" onClick={confirmCheckout}>
            确认并支付
          </button>
        </section>

        <section className="progress-panel">
          <div className="panel-title">
            <h2>执行进度</h2>
            {selectedOrder?.status === "running" && (
              <button type="button" onClick={cancelSelectedOrder}>
                取消订单
              </button>
            )}
          </div>
          {timeline.map((step) => (
            <div className={`timeline-item ${step.state}`} key={step.key || step.label}>
              <span className="timeline-dot" />
              <strong>{step.label}</strong>
              <time>{step.timestamp ? formatDateTime(step.timestamp) : "待推进"}</time>
              <p>{step.state === "current" ? "当前正在处理" : step.state === "done" ? "已完成" : "等待进入"}</p>
            </div>
          ))}
        </section>

        <section className="delivery-panel">
          <div className="panel-title">
            <h2>交付物</h2>
            <button type="button" onClick={() => setSelectedStep("artifact")}>
              查看
            </button>
          </div>
          {inspectorArtifacts.length > 0 ? (
            inspectorArtifacts.map((artifact) => (
              <div className="file-row" key={artifact.id}>
                <span className="file-icon">PDF</span>
                <div>
                  <strong>{artifact.fileName}</strong>
                  <small>{Math.max(1, Math.round((artifact.sizeBytes || 0) / 1024))} KB · {artifact.fileType}</small>
                </div>
                <button type="button" onClick={() => shareArtifact(artifact.id)}>
                  分享
                </button>
                <button type="button" onClick={() => downloadArtifact(artifact.id)}>
                  下载
                </button>
              </div>
            ))
          ) : (
            <p className="muted-copy">订单交付后会在这里出现报告预览、下载和分享。</p>
          )}
        </section>
      </section>
    );
  }

  function renderOrders() {
    return (
      <section className="main-column">
        <div className="section-heading">
          <span>我的订单</span>
          <h1>订单执行记录</h1>
        </div>
        <SegmentControl
          items={orderFilters}
          label="订单状态筛选"
          value={orderFilter}
          onChange={setOrderFilter}
        />
        <div className="order-list">
          {userOrders.length > 0 ? (
            userOrders.map((order) => (
              <button
                className={selectedOrder?.id === order.id ? "selected" : ""}
                key={order.id}
                type="button"
                onClick={() => chooseOrder(order)}
              >
                <span>
                  <strong>{order.requirement?.prompt || "本次任务"}</strong>
                  <small>{order.agentName || "Agent"} · {formatDateTime(order.createdAt)}</small>
                </span>
                <em>{statusLabel(order.status)}</em>
              </button>
            ))
          ) : (
            <section className="empty-panel">暂无订单。回到工作台提交一次 Agent 任务。</section>
          )}
        </div>
        {selectedOrder && (
          <section className="order-detail-panel">
            <div className="panel-title">
              <h2>订单详情</h2>
              {selectedOrder.status === "running" && (
                <button type="button" onClick={cancelSelectedOrder}>
                  取消订单
                </button>
              )}
            </div>
            <dl>
              <div><dt>订单号</dt><dd>{selectedOrder.id}</dd></div>
              <div><dt>关联 Agent</dt><dd>{selectedOrder.agentName}</dd></div>
              <div><dt>订单状态</dt><dd>{statusLabel(selectedOrder.status)}</dd></div>
              <div><dt>预计完成</dt><dd>{formatDateTime(selectedOrder.estimatedCompletionAt)}</dd></div>
              <div><dt>本次扣减</dt><dd>{selectedOrder.usageCost || 1} 次</dd></div>
              <div><dt>总金额</dt><dd>{formatMoney(selectedOrder.amountCents)}</dd></div>
            </dl>
          </section>
        )}
      </section>
    );
  }

  function renderBilling() {
    return (
      <section className="main-column">
        <div className="section-heading">
          <span>用量与账单</span>
          <h1>{currentBalance} 次可用</h1>
        </div>
        <section className="usage-hero">
          <div>
            <span>剩余次数</span>
            <strong>{currentBalance}</strong>
            <small>每次 Agent 执行扣减 1 次</small>
          </div>
          <button className="primary-button" type="button" onClick={buyUses}>
            购买 10 次
          </button>
        </section>
        <section className="panel ledger-panel">
          <div className="panel-title">
            <h2>最近使用记录</h2>
            <span>{recentLedger.length} 条</span>
          </div>
          {recentLedger.length > 0 ? (
            recentLedger.map((entry) => (
              <div className="ledger-row" key={entry.id}>
                <span>
                  <strong>{ledgerLabel(entry.type)}</strong>
                  <small>{formatDateTime(entry.createdAt)}</small>
                </span>
                <em>{entry.delta > 0 ? `+${entry.delta}` : entry.delta} 次</em>
              </div>
            ))
          ) : (
            <p className="muted-copy">暂无用量记录。</p>
          )}
        </section>
      </section>
    );
  }

  function renderAccount() {
    return (
      <section className="main-column">
        <div className="section-heading">
          <span>账户</span>
          <h1>{currentUser ? "账户已连接" : "登录 Oneshot"}</h1>
        </div>
        {currentUser ? (
          <section className="login-panel">
            <div className="account-avatar">{currentUser.displayName?.slice(0, 1) || "U"}</div>
            <div>
              <strong>{currentUser.displayName || currentUser.email || "Oneshot 用户"}</strong>
              <p>{authProviderLabel}授权已连接</p>
            </div>
            <button className="outline-button" type="button" onClick={logout}>
              退出登录
            </button>
          </section>
        ) : (
          <section className="login-panel">
            <div>
              <strong>{authStatus === "checking" ? "正在检查账号" : "登录后开始使用 Agent 服务"}</strong>
              <p>登录后可以购买次数、创建订单和查看交付物。</p>
            </div>
            <button className="primary-button" type="button" disabled={authStatus === "checking"} onClick={() => login("wechat")}>
              微信授权登录
            </button>
            <button className="outline-button" type="button" disabled={authStatus === "checking"} onClick={() => login("google")}>
              Google 邮箱登录
            </button>
          </section>
        )}
      </section>
    );
  }

  function renderMainView() {
    if (activeView === "orders") return renderOrders();
    if (activeView === "billing") return renderBilling();
    if (activeView === "account") return renderAccount();
    return renderWorkbench();
  }

  return (
    <main className="app-shell">
      <aside className="sidebar" aria-label="主导航">
        <div className="brand">
          <strong>oneshot</strong>
          <span>Agent 服务市场</span>
        </div>

        <nav className="nav-block">
          {navigation.map((item) => (
            <button
              className={activeView === item.id ? "active" : ""}
              key={item.id}
              type="button"
              onClick={() => setActiveView(item.id)}
            >
              <span className="nav-icon">{item.icon}</span>
              {item.label}
            </button>
          ))}
        </nav>

        <div className="sidebar-footer">
          {currentUser ? (
            <section className="auth-card">
              <span className="auth-avatar">{currentUser.displayName?.slice(0, 1) || "U"}</span>
              <div>
                <strong>{currentUser.displayName || currentUser.email || "Oneshot 用户"}</strong>
                <small>{authProviderLabel}已连接</small>
              </div>
            </section>
          ) : (
            <section className="auth-card compact">
              <strong>登录账号</strong>
              <button type="button" onClick={() => login("wechat")}>微信登录</button>
              <button type="button" onClick={() => login("google")}>Google</button>
            </section>
          )}
        </div>
      </aside>

      <section className="workspace">
        <header className="topbar">
          <button className="back-link" type="button" onClick={() => setActiveView("workbench")}>
            返回工作台
          </button>
          <div className="top-actions">
            <button className="ghost-link" type="button" onClick={() => showToast("帮助中心暂未接入")}>
              帮助中心
            </button>
            <div className="inspector-tool">
              <button
                className={`inspector-trigger ${inspectorMenuOpen ? "active" : ""}`}
                type="button"
                aria-haspopup="menu"
                aria-expanded={inspectorMenuOpen}
                onClick={() => setInspectorMenuOpen((value) => !value)}
              >
                <span aria-hidden="true">▣</span>
                <small>⌄</small>
              </button>
              {inspectorMenuOpen && renderInspectorMenu()}
            </div>
          </div>
        </header>

        <div className={`content-grid ${inspectorPanelOpen ? "inspector-open" : "inspector-closed"}`}>
          {renderMainView()}

          {inspectorPanelOpen ? renderInspectorPanel() : null}
        </div>
      </section>

      {toast && (
        <div className="toast" role="status">
          {toast}
        </div>
      )}
    </main>
  );
}
