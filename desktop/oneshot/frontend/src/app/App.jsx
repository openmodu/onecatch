import { useEffect, useMemo, useRef, useState } from "react";
import {
  AgentBinding,
  ArtifactBinding,
  AuthBinding,
  BillingBinding,
  ConversationBinding,
  OrderBinding,
} from "../../bindings/github.com/openmodu/oneshot/desktop/oneshot/bindings/index.js";

const iconPaths = {
  workbench: (
    <>
      <rect x="4" y="4" width="6.5" height="6.5" rx="1.4" />
      <rect x="13.5" y="4" width="6.5" height="6.5" rx="1.4" />
      <rect x="4" y="13.5" width="6.5" height="6.5" rx="1.4" />
      <rect x="13.5" y="13.5" width="6.5" height="6.5" rx="1.4" />
    </>
  ),
  orders: (
    <>
      <path d="M8 6h13M8 12h13M8 18h13" />
      <path d="M3 6h.01M3 12h.01M3 18h.01" />
    </>
  ),
  billing: (
    <>
      <path d="M3 3v18h18" />
      <path d="M18 17V9M13 17V5M8 17v-3" />
    </>
  ),
  account: (
    <>
      <circle cx="12" cy="12" r="9" />
      <circle cx="12" cy="9" r="3" />
      <path d="M6.8 18.6a5.7 5.7 0 0 1 10.4 0" />
    </>
  ),
  preview: (
    <>
      <path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7Z" />
      <circle cx="12" cy="12" r="3" />
    </>
  ),
  detail: (
    <>
      <path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z" />
      <path d="M15 2v5h5M10 9H8M16 13H8M16 17H8" />
    </>
  ),
  order: (
    <>
      <rect x="6" y="4" width="12" height="16" rx="2" />
      <path d="M9 8h6M9 12h6M9 16h4" />
    </>
  ),
  records: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 7v5l3 2" />
    </>
  ),
  progress: <path d="M22 12h-4l-3 9L9 3l-3 9H2" />,
  panels: (
    <>
      <rect x="3.5" y="5" width="17" height="14" rx="2.2" />
      <path d="M14.5 5v14" />
    </>
  ),
  chevronLeft: <path d="m15 18-6-6 6-6" />,
};

function Icon({ name }) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.55"
      strokeLinecap="round"
      strokeLinejoin="round"
      vectorEffect="non-scaling-stroke"
      aria-hidden="true"
    >
      {iconPaths[name]}
    </svg>
  );
}

const navigation = [
  { id: "workbench", label: "工作台", icon: "workbench" },
  { id: "orders", label: "我的订单", icon: "orders" },
  { id: "billing", label: "用量账单", icon: "billing" },
  { id: "account", label: "账户", icon: "account" },
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
  { id: "preview", label: "预览", icon: "preview", shortcut: "⌘1" },
  { id: "detail", label: "明细", icon: "detail", shortcut: "⌘2" },
  { id: "order", label: "订单", icon: "order", shortcut: "⌘3" },
  { id: "records", label: "记录", icon: "records", shortcut: "⌘4" },
  { id: "progress", label: "进度", icon: "progress", shortcut: "⌘5" },
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

function isWailsRuntime() {
  if (typeof window === "undefined") return false;
  return Boolean(window._wails?.environment?.OS);
}

function canUseFallback() {
  if (!import.meta.env.DEV || typeof window === "undefined") return false;
  if (window.__ONESOT_DISABLE_FIXTURE__ || window.__ONESHOT_DISABLE_FIXTURE__) return false;
  return !isWailsRuntime();
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
  const [toast, setToast] = useState(null);
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
  const inspectorToolRef = useRef(null);
  const [conversation, setConversation] = useState(null);
  const [composer, setComposer] = useState("");
  const [conversationBusy, setConversationBusy] = useState(false);
  const [showAgentPicker, setShowAgentPicker] = useState(false);
  const threadEndRef = useRef(null);

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
    if (canUseFallback()) {
      setAgents(fallbackAgents);
      setSelectedAgentId((current) =>
        fallbackAgents.some((agent) => agent.id === current) ? current : fallbackAgents[0].id,
      );
      setAgentStatus("fixture");
      return undefined;
    }

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
    if (canUseFallback()) {
      setCurrentUser(null);
      setAuthProvider("");
      setAuthStatus("signed-out");
      setBalance({ remaining: 12 });
      setLedger([
        { id: "ledger-demo-1", type: "purchase", delta: 10, balanceAfter: 12, createdAt: "2026-06-10T09:00:00+08:00" },
        { id: "ledger-demo-2", type: "debit", delta: -1, balanceAfter: 11, createdAt: "2026-06-10T10:31:00+08:00" },
      ]);
      setUserOrders(fallbackOrders);
      setSelectedOrderId(fallbackOrders[0].id);
      return undefined;
    }

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
    const timer = window.setInterval(() => refreshOrders(orderFilter), 8000);
    return () => window.clearInterval(timer);
  }, [currentUser?.id, orderFilter]);

  useEffect(() => {
    function onKeyDown(event) {
      if (event.metaKey && !event.ctrlKey && !event.altKey && !event.shiftKey) {
        const index = Number(event.key) - 1;
        if (index >= 0 && index < inspectorPanels.length) {
          event.preventDefault();
          openInspectorPanel(inspectorPanels[index].id);
        }
        return;
      }
      if (event.key === "Escape") {
        setInspectorMenuOpen(false);
      }
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);

  useEffect(() => {
    if (!inspectorMenuOpen) return undefined;
    function onPointerDown(event) {
      if (!inspectorToolRef.current?.contains(event.target)) {
        setInspectorMenuOpen(false);
      }
    }
    window.addEventListener("pointerdown", onPointerDown);
    return () => window.removeEventListener("pointerdown", onPointerDown);
  }, [inspectorMenuOpen]);

  useEffect(() => {
    threadEndRef.current?.scrollIntoView({ behavior: "smooth", block: "end" });
  }, [conversation?.messages?.length, selectedOrder?.status, inspectorArtifacts.length]);

  useEffect(() => {
    let alive = true;
    if (!currentUser?.id || !selectedOrder?.id || selectedOrder.status !== "delivered") {
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
  }, [currentUser?.id, selectedOrder?.id, selectedOrder?.status]);

  function toastTone(message) {
    if (/失败|错误|无法|请先|不足|不存在|unauthenticated/i.test(message)) return "error";
    if (/已|成功|通过|完成|生成|增加/.test(message)) return "success";
    return "info";
  }

  function showToast(message, tone) {
    const nextTone = tone || toastTone(message);
    setToast({ message, tone: nextTone });
    window.clearTimeout(showToast.timer);
    showToast.timer = window.setTimeout(() => setToast(null), nextTone === "error" ? 3600 : 2400);
  }

  async function refreshBilling() {
    if (canUseFallback()) {
      setBalance((value) => value ?? { remaining: 12 });
      setLedger((items) =>
        items.length > 0
          ? items
          : [
              { id: "ledger-demo-1", type: "purchase", delta: 10, balanceAfter: 12, createdAt: "2026-06-10T09:00:00+08:00" },
              { id: "ledger-demo-2", type: "debit", delta: -1, balanceAfter: 11, createdAt: "2026-06-10T10:31:00+08:00" },
            ],
      );
      return;
    }

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
    if (canUseFallback()) {
      const nextOrders = status
        ? fallbackOrders.filter((order) => order.status === status)
        : fallbackOrders;
      setUserOrders((items) => (items.length > 0 ? items : nextOrders));
      setSelectedOrderId((current) =>
        nextOrders.some((order) => order.id === current) ? current : nextOrders[0]?.id || "",
      );
      return;
    }

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
      setAuthStatus("signed-out");
      showToast(error?.message || "登录失败，请稍后重试");
    }
  }

  async function logout() {
    if (canUseFallback()) {
      setCurrentUser(null);
      setAuthProvider("");
      window.localStorage.removeItem("oneshot.authProvider");
      setAuthStatus("signed-out");
      showToast("已退出本地预览登录态");
      return;
    }

    try {
      await AuthBinding.Logout();
      setCurrentUser(null);
      setAuthProvider("");
      window.localStorage.removeItem("oneshot.authProvider");
      setAuthStatus("signed-out");
      showToast("已退出登录");
    } catch (error) {
      showToast(error?.message || "退出登录失败");
    }
  }

  async function buyUses() {
    if (!currentUser && !canUseFallback()) {
      showToast("请先登录后再购买次数");
      setActiveView("account");
      return;
    }
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

    try {
      await BillingBinding.StartPurchase({ planId: "uses_10" });
      await refreshBilling();
      showToast("已购买 10 次");
    } catch (error) {
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
      showToast(error?.message || "创建订单失败");
    }
  }

  async function cancelSelectedOrder() {
    if (!selectedOrder?.id) return;
    if (!currentUser?.id && !canUseFallback()) {
      showToast("请先登录后再取消订单");
      setActiveView("account");
      return;
    }
    if (canUseFallback()) {
      setUserOrders((items) =>
        items.map((order) =>
          order.id === selectedOrder.id ? { ...order, status: "cancelled" } : order,
        ),
      );
      showToast("本地预览已取消订单");
      return;
    }

    try {
      await OrderBinding.CancelOrder(selectedOrder.id);
      await refreshOrders(orderFilter);
      showToast("订单已取消");
    } catch (error) {
      showToast(error?.message || "取消失败");
    }
  }

  async function downloadArtifact(artifactId) {
    if (!currentUser?.id && !canUseFallback()) {
      showToast("请先登录后再下载交付物");
      setActiveView("account");
      return;
    }
    if (canUseFallback()) {
      showToast("本地预览已模拟下载");
      return;
    }

    try {
      const download = await ArtifactBinding.DownloadArtifact(artifactId);
      if (download.filePath) {
        await ArtifactBinding.ShowInFolder(download.filePath);
      }
      showToast(download.filePath ? `已下载到 ${download.filePath}` : "已下载交付物");
    } catch (error) {
      showToast(error?.message || "下载失败");
    }
  }

  async function shareArtifact(artifactId) {
    if (!currentUser?.id && !canUseFallback()) {
      showToast("请先登录后再分享交付物");
      setActiveView("account");
      return;
    }
    if (canUseFallback()) {
      showToast("本地预览已模拟分享");
      return;
    }

    try {
      const share = await ArtifactBinding.ShareArtifact(artifactId);
      await navigator.clipboard?.writeText?.(share.url);
      showToast("分享链接已生成");
    } catch (error) {
      showToast(error?.message || "分享失败");
    }
  }

  function localConversationMessage(role, kind, text) {
    return {
      id: `local-msg-${Date.now()}-${Math.round(Math.random() * 1e6)}`,
      role,
      kind,
      text,
      createdAt: new Date().toISOString(),
    };
  }

  async function startConversationFor(agent) {
    if (!agent) return;
    if (!currentUser?.id && !canUseFallback()) {
      showToast("请先登录后再开始会话");
      setActiveView("account");
      return;
    }
    setActiveView("workbench");
    setSelectedAgentId(agent.id);
    setSelectedOrderId("");
    setShowAgentPicker(false);
    setComposer("");
    setConversationBusy(true);
    try {
      const conv = await ConversationBinding.StartConversation(agent.id);
      setConversation(conv);
    } catch (error) {
      if (canUseFallback()) {
        setConversation({
          id: `local-conv-${Date.now()}`,
          agentId: agent.id,
          agentName: agent.name,
          status: "active",
          messages: [
            localConversationMessage(
              "agent",
              "text",
              `你好，我是${agent.name}。${agent.description || ""}\n请描述你的任务，我会先给出扣次确认。`,
            ),
          ],
        });
      } else {
        showToast(error?.message || "无法开始会话");
      }
    } finally {
      setConversationBusy(false);
    }
  }

  async function sendConversationMessage() {
    const text = composer.trim();
    if (!text || !conversation) return;
    if (!currentUser?.id && !canUseFallback()) {
      showToast("请先登录后再发送消息");
      setActiveView("account");
      return;
    }
    setComposer("");
    setConversationBusy(true);
    try {
      const conv = await ConversationBinding.PostMessage(conversation.id, text);
      setConversation(conv);
    } catch (error) {
      if (canUseFallback()) {
        setConversation((prev) => ({
          ...prev,
          status: "awaiting_confirm",
          messages: [
            ...prev.messages,
            localConversationMessage("user", "text", text),
            localConversationMessage(
              "agent",
              "checkout",
              `我已理解你的任务。本次执行将扣减 ${selectedAgent?.priceUses || 1} 次，确认后开始。`,
            ),
          ],
        }));
      } else {
        showToast(error?.message || "发送失败");
      }
    } finally {
      setConversationBusy(false);
    }
  }

  async function confirmConversationCheckout() {
    if (!conversation || conversationBusy) return;
    if (!currentUser?.id && !canUseFallback()) {
      showToast("请先登录后再确认支付");
      setActiveView("account");
      return;
    }
    setConversationBusy(true);
    try {
      const conv = await ConversationBinding.ConfirmCheckout(conversation.id);
      setConversation(conv);
      if (conv.orderId) setSelectedOrderId(conv.orderId);
      await Promise.all([refreshBilling(), refreshOrders("")]);
      showToast("已开始执行");
    } catch (error) {
      if (canUseFallback()) {
        const orderId = `ORD${Date.now().toString().slice(-12)}`;
        const lastUser = [...(conversation.messages || [])]
          .reverse()
          .find((m) => m.role === "user");
        const order = {
          id: orderId,
          agentName: conversation.agentName,
          requirement: { prompt: lastUser?.text || "本次任务" },
          status: "running",
          usageCost: selectedAgent?.priceUses || 1,
          amountCents: selectedAgent?.priceCents,
          createdAt: new Date().toISOString(),
          progress: [],
        };
        setUserOrders((items) => [order, ...items]);
        setSelectedOrderId(orderId);
        setConversation((prev) => ({
          ...prev,
          status: "running",
          orderId,
          messages: [
            ...prev.messages,
            localConversationMessage("system", "text", `已开始执行，订单号 ${orderId}。进度和交付物会在这里更新。`),
          ],
        }));
        setBalance((value) => ({ remaining: Math.max(0, (value?.remaining ?? 12) - 1) }));
        showToast("本地预览已开始执行");
      } else {
        showToast(error?.message || "确认失败");
      }
    } finally {
      setConversationBusy(false);
    }
  }

  function chooseAgent(agent) {
    startConversationFor(agent);
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
            <span className="menu-icon"><Icon name={panel.icon} /></span>
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
    const convOrder = conversation?.orderId ? selectedOrder : null;
    const awaiting = conversation?.status === "awaiting_confirm";

    const renderChatMessage = (message) => {
      if (message.kind === "checkout") {
        return (
          <div className="chat-row agent" key={message.id}>
            <div className="chat-bubble chat-card chat-checkout">
              <p>{message.text}</p>
              <div className="chat-checkout-actions">
                {awaiting ? (
                  <button
                    className="primary-button"
                    type="button"
                    disabled={conversationBusy}
                    onClick={confirmConversationCheckout}
                  >
                    确认并支付
                  </button>
                ) : (
                  <span className="chat-confirmed">已确认</span>
                )}
              </div>
            </div>
          </div>
        );
      }
      const side = message.role === "user" ? "user" : "agent";
      return (
        <div className={`chat-row ${side}`} key={message.id}>
          <div className="chat-bubble">{message.text}</div>
        </div>
      );
    };

    const picker = (
      <div className="agent-picker">
        <SegmentControl
          items={categories}
          label="Agent 分类"
          value={activeCategory}
          onChange={setActiveCategory}
        />
        <div className="agent-strip" aria-label="选择 Agent">
          {visibleAgents.map((agent) => (
            <button
              className={conversation?.agentId === agent.id ? "selected" : ""}
              key={agent.id}
              type="button"
              onClick={() => startConversationFor(agent)}
            >
              <span>{agent.name.slice(0, 1)}</span>
              <strong>{agent.name}</strong>
              <em>{formatMoney(agent.priceCents)}</em>
            </button>
          ))}
          {visibleAgents.length === 0 && (
            <p className="muted-copy">
              {agentStatus === "loading" ? "正在加载 Agent 目录" : "该分类暂无可用 Agent"}
            </p>
          )}
        </div>
      </div>
    );

    if (!conversation) {
      return (
        <section className="workbench-chat">
          <div className="chat-intro">
            <h1>选择一个 Agent 开始对话</h1>
            <p className="muted-copy">
              描述你的任务，确认扣次后开始执行；进度与交付物都在对话里呈现。
            </p>
          </div>
          {picker}
        </section>
      );
    }

    return (
      <section className="workbench-chat">
        <header className="chat-header">
          <div className="chat-agent">
            <span className="agent-mark-sm" aria-hidden="true">
              {conversation.agentName?.slice(0, 1) || "A"}
            </span>
            <div>
              <strong>{conversation.agentName}</strong>
              <small>
                {selectedAgent
                  ? `${formatMoney(selectedAgent.priceCents)} / 次 · 余额 ${currentBalance} 次`
                  : `余额 ${currentBalance} 次`}
              </small>
            </div>
          </div>
          <button className="soft-button" type="button" onClick={() => setShowAgentPicker((v) => !v)}>
            {showAgentPicker ? "收起" : "切换 Agent"}
          </button>
        </header>

        {showAgentPicker && picker}

        <div className="chat-thread" aria-label="对话">
          {conversation.messages.map(renderChatMessage)}

          {convOrder && (
            <div className="chat-row agent">
              <div className="chat-bubble chat-card">
                <div className="chat-card-title">
                  <span>执行进度</span>
                  {convOrder.status === "running" && (
                    <button type="button" onClick={cancelSelectedOrder}>
                      取消
                    </button>
                  )}
                </div>
                <div className="chat-timeline">
                  {timeline.map((step) => (
                    <div className={`chat-timeline-row ${step.state}`} key={step.key || step.label}>
                      <span className="timeline-dot" />
                      <strong>{step.label}</strong>
                      <time>{step.timestamp ? formatDateTime(step.timestamp) : "待推进"}</time>
                    </div>
                  ))}
                </div>
              </div>
            </div>
          )}

          {convOrder && inspectorArtifacts.length > 0 && (
            <div className="chat-row agent">
              <div className="chat-bubble chat-card">
                <div className="chat-card-title">
                  <span>交付物</span>
                </div>
                {inspectorArtifacts.map((artifact) => (
                  <div className="file-row" key={artifact.id}>
                    <span className="file-icon">PDF</span>
                    <div>
                      <strong>{artifact.fileName}</strong>
                      <small>
                        {Math.max(1, Math.round((artifact.sizeBytes || 0) / 1024))} KB · {artifact.fileType}
                      </small>
                    </div>
                    <button type="button" onClick={() => shareArtifact(artifact.id)}>
                      分享
                    </button>
                    <button type="button" onClick={() => downloadArtifact(artifact.id)}>
                      下载
                    </button>
                  </div>
                ))}
              </div>
            </div>
          )}

          <div ref={threadEndRef} />
        </div>

        <div className="chat-composer">
          <textarea
            aria-label="输入任务"
            placeholder={awaiting ? "可继续补充或修改任务…" : "描述你要完成的任务…"}
            value={composer}
            onChange={(event) => setComposer(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter" && (event.metaKey || event.ctrlKey)) {
                event.preventDefault();
                sendConversationMessage();
              }
            }}
          />
          <button
            className="primary-button"
            type="button"
            disabled={conversationBusy || !composer.trim()}
            onClick={sendConversationMessage}
          >
            发送
          </button>
        </div>
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
              <span className="nav-icon"><Icon name={item.icon} /></span>
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
            <Icon name="chevronLeft" />
            返回工作台
          </button>
          <div className="top-actions">
            <button className="ghost-link" type="button" onClick={() => showToast("帮助中心暂未接入")}>
              帮助中心
            </button>
            <div className="inspector-tool" ref={inspectorToolRef}>
              <button
                className={`inspector-trigger ${inspectorMenuOpen ? "active" : ""}`}
                type="button"
                aria-haspopup="menu"
                aria-expanded={inspectorMenuOpen}
                onClick={() => setInspectorMenuOpen((value) => !value)}
              >
                <span aria-hidden="true"><Icon name="panels" /></span>
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
        <div className={`toast ${toast.tone}`} role={toast.tone === "error" ? "alert" : "status"}>
          <span className="toast-dot" aria-hidden="true" />
          <span>{toast.message}</span>
        </div>
      )}
    </main>
  );
}
