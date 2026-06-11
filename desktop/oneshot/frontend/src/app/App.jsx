import { useEffect, useMemo, useState } from "react";
import {
  AgentBinding,
  ArtifactBinding,
  AuthBinding,
  BillingBinding,
  OrderBinding,
} from "../../bindings/github.com/openmodu/oneshot/desktop/oneshot/bindings/index.js";

const sections = [
  { id: "agents", label: "Agent", icon: "A" },
  { id: "orders", label: "订单", icon: "O" },
  { id: "account", label: "账户", icon: "U" },
];

const categories = [
  { id: "all", label: "全部" },
  { id: "content", label: "内容" },
  { id: "data", label: "数据" },
  { id: "marketing", label: "营销" },
  { id: "design", label: "设计" },
  { id: "dev", label: "开发" },
  { id: "office", label: "办公" },
  { id: "research", label: "研究" },
];

const orderFilters = [
  { id: "", label: "全部" },
  { id: "running", label: "执行中" },
  { id: "delivered", label: "已交付" },
  { id: "pending_payment", label: "待支付" },
];

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

function NavIcon({ children }) {
  return (
    <span className="nav-icon" aria-hidden="true">
      {children}
    </span>
  );
}

function SegmentControl({ items, value, onChange, label }) {
  return (
    <div className="segment-control" aria-label={label}>
      {items.map((item) => (
        <button
          className={value === item.id ? "is-selected" : ""}
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
  const [agentStatus, setAgentStatus] = useState("loading");
  const [currentUser, setCurrentUser] = useState(null);
  const [authProvider, setAuthProvider] = useState("");
  const [authStatus, setAuthStatus] = useState("checking");
  const [toast, setToast] = useState("");
  const [activeSection, setActiveSection] = useState("agents");
  const [activeCategory, setActiveCategory] = useState("all");
  const [selectedAgentId, setSelectedAgentId] = useState("");
  const [selectedOrderId, setSelectedOrderId] = useState("");
  const [orderFilter, setOrderFilter] = useState("");
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
  const authProviderLabel = authProvider === "google" ? "Google 邮箱" : "微信";
  const currentBalance = balance?.remaining ?? 0;
  const recentLedger = ledger.slice(-6).reverse();
  const currentTimeline =
    selectedOrder?.progress?.length > 0
      ? selectedOrder.progress
      : [
          { state: "next", label: "需求已提交", timestamp: null },
          { state: "next", label: "扣次确认", timestamp: null },
          { state: "next", label: "执行中", timestamp: null },
          { state: "next", label: "交付物生成中", timestamp: null },
          { state: "next", label: "已交付", timestamp: null },
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
    if (!currentUser?.id) {
      setBalance(null);
      setLedger([]);
      setUserOrders([]);
      setSelectedOrderId("");
      setArtifacts([]);
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
        setArtifacts([]);
      });
    return () => {
      alive = false;
    };
  }, [selectedOrder?.id, selectedOrder?.status]);

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
      });
    return () => {
      alive = false;
    };
  }, []);

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
      showToast(error?.message || "退出登录失败");
    }
  }

  async function buyUses() {
    if (!currentUser) {
      showToast("请先登录后再购买次数");
      setActiveSection("account");
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
    if (!currentUser) {
      showToast("请先登录后再创建订单");
      setActiveSection("account");
      return;
    }
    if (!requirement.trim()) {
      showToast("请先填写任务需求");
      return;
    }
    try {
      const order = await OrderBinding.CreateOrder({
        agentId: selectedAgent.id,
        requirement: { prompt: requirement },
      });
      setSelectedOrderId(order.id);
      setActiveSection("orders");
      setOrderFilter("");
      await Promise.all([refreshBilling(), refreshOrders("")]);
      showToast("已开始执行");
    } catch (error) {
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
      showToast(`已下载到 ${download.filePath}`);
    } catch (error) {
      showToast(error?.message || "下载失败");
    }
  }

  async function shareArtifact(artifactId) {
    try {
      const share = await ArtifactBinding.ShareArtifact(artifactId);
      await navigator.clipboard?.writeText?.(share.url);
      showToast("分享链接已生成");
    } catch (error) {
      showToast(error?.message || "分享失败");
    }
  }

  function renderMiddlePane() {
    if (activeSection === "orders") {
      return (
        <>
          <header className="pane-header">
            <p>我的订单</p>
            <h1>执行记录</h1>
          </header>
          <SegmentControl
            items={orderFilters}
            label="订单状态筛选"
            value={orderFilter}
            onChange={setOrderFilter}
          />
          <div className="item-list">
            {userOrders.length > 0 ? userOrders.map((order) => (
              <button
                className={`list-item ${selectedOrder?.id === order.id ? "is-selected" : ""}`}
                key={order.id}
                type="button"
                onClick={() => setSelectedOrderId(order.id)}
              >
                <span>
                  <strong>{order.requirement?.prompt || "本次任务"}</strong>
                  <small>{order.agentName || "Agent"} · {statusLabel(order.status)}</small>
                </span>
                <em>{formatDateTime(order.createdAt)}</em>
              </button>
            )) : (
              <div className="empty-state">暂无订单</div>
            )}
          </div>
        </>
      );
    }

    if (activeSection === "account") {
      return (
        <>
          <header className="pane-header">
            <p>账户</p>
            <h1>{currentUser ? "账户与用量" : "登录账户"}</h1>
          </header>
          <div className="account-summary-list">
            <button className="list-item is-selected" type="button">
              <span>
                <strong>{currentUser?.displayName || currentUser?.email || "未登录"}</strong>
                <small>{currentUser ? `${authProviderLabel}授权已连接` : "登录后可创建订单和查看交付物"}</small>
              </span>
            </button>
            <button className="list-item" type="button" onClick={buyUses}>
              <span>
                <strong>{currentBalance} 次可用</strong>
                <small>购买 10 次使用额度</small>
              </span>
            </button>
          </div>
        </>
      );
    }

    return (
      <>
        <header className="pane-header">
          <p>Agent 市场</p>
          <h1>选择一个服务</h1>
        </header>
        <SegmentControl
          items={categories}
          label="Agent 分类"
          value={activeCategory}
          onChange={setActiveCategory}
        />
        <div className="item-list">
          {visibleAgents.length > 0 ? visibleAgents.map((agent) => (
            <button
              className={`list-item agent-list-item ${selectedAgent?.id === agent.id ? "is-selected" : ""}`}
              key={agent.id}
              type="button"
              onClick={() => setSelectedAgentId(agent.id)}
            >
              <span>
                <strong>{agent.name}</strong>
                <small>{agent.description}</small>
              </span>
              <em>{formatMoney(agent.priceCents)}</em>
            </button>
          )) : (
            <div className="empty-state">
              {agentStatus === "loading" ? "正在加载 Agent" : "该分类暂无 Agent"}
            </div>
          )}
        </div>
      </>
    );
  }

  function renderDetailPane() {
    if (activeSection === "orders") {
      return (
        <section className="detail-panel">
          <header className="detail-toolbar">
            <div>
              <p>订单详情</p>
              <h2>{selectedOrder?.agentName || "选择一个订单"}</h2>
            </div>
            {selectedOrder?.status === "running" && (
              <button className="ghost-button" type="button" onClick={cancelSelectedOrder}>
                取消订单
              </button>
            )}
          </header>

          {selectedOrder ? (
            <>
              <div className="detail-card status-card">
                <span>{statusLabel(selectedOrder.status)}</span>
                <strong>{selectedOrder.requirement?.prompt || "本次任务"}</strong>
                <small>预计完成：{formatDateTime(selectedOrder.estimatedCompletionAt)}</small>
              </div>

              <section className="timeline-card">
                <h3>执行进度</h3>
                {currentTimeline.map((step) => (
                  <div className={`timeline-row ${step.state}`} key={step.key || step.label}>
                    <span className="timeline-dot" />
                    <strong>{step.label}</strong>
                    <time>{step.timestamp ? formatDateTime(step.timestamp) : "待推进"}</time>
                  </div>
                ))}
              </section>

              <section className="detail-card artifact-card">
                <div className="card-heading">
                  <h3>交付物</h3>
                  <span>{artifacts[0] ? artifacts[0].fileType : "PDF"}</span>
                </div>
                {artifacts[0] ? (
                  <div className="artifact-row">
                    <span>
                      <strong>{artifacts[0].fileName}</strong>
                      <small>{Math.max(1, Math.round((artifacts[0].sizeBytes || 0) / 1024))} KB</small>
                    </span>
                    <div className="inline-actions">
                      <button type="button" onClick={() => shareArtifact(artifacts[0].id)}>分享</button>
                      <button type="button" onClick={() => downloadArtifact(artifacts[0].id)}>下载</button>
                    </div>
                  </div>
                ) : (
                  <p className="muted-copy">订单交付后会在这里出现报告下载。</p>
                )}
              </section>
            </>
          ) : (
            <div className="empty-detail">从中间列表选择一个订单。</div>
          )}
        </section>
      );
    }

    if (activeSection === "account") {
      return (
        <section className="detail-panel">
          <header className="detail-toolbar">
            <div>
              <p>账户</p>
              <h2>{currentUser ? currentUser.displayName || currentUser.email : "登录 Oneshot"}</h2>
            </div>
            {currentUser && (
              <button className="ghost-button" type="button" onClick={logout}>
                退出
              </button>
            )}
          </header>

          {currentUser ? (
            <>
              <div className="usage-hero">
                <span>剩余次数</span>
                <strong>{currentBalance}</strong>
                <button type="button" onClick={buyUses}>购买 10 次</button>
              </div>
              <section className="detail-card ledger-card">
                <div className="card-heading">
                  <h3>最近用量</h3>
                  <span>{recentLedger.length} 条</span>
                </div>
                {recentLedger.length > 0 ? recentLedger.map((entry) => (
                  <div className="ledger-row" key={entry.id}>
                    <span>{entry.type === "purchase" ? "购买次数" : "执行扣次"}</span>
                    <strong>{entry.delta > 0 ? `+${entry.delta}` : entry.delta}</strong>
                  </div>
                )) : (
                  <p className="muted-copy">暂无用量记录。</p>
                )}
              </section>
            </>
          ) : (
            <div className="login-card">
              <p>{authStatus === "checking" ? "正在检查账号" : "登录后开始使用 Agent 服务"}</p>
              <button type="button" disabled={authStatus === "checking"} onClick={() => login("wechat")}>
                微信授权登录
              </button>
              <button className="ghost-button" type="button" disabled={authStatus === "checking"} onClick={() => login("google")}>
                Google 邮箱登录
              </button>
            </div>
          )}
        </section>
      );
    }

    return (
      <section className="detail-panel">
        <header className="detail-toolbar">
          <div>
            <p>Agent 详情</p>
            <h2>{selectedAgent?.name || "选择一个 Agent"}</h2>
          </div>
          <button className="balance-pill" type="button" onClick={buyUses}>
            {currentBalance} 次
          </button>
        </header>

        {selectedAgent ? (
          <>
            <div className="agent-hero">
              <div className="agent-avatar" aria-hidden="true">{selectedAgent.name?.slice(0, 1) || "A"}</div>
              <div>
                <h3>{selectedAgent.name}</h3>
                <p>{selectedAgent.description}</p>
                <div className="meta-row">
                  <span>评分 {selectedAgent.rating}</span>
                  <span>成交 {formatDealCount(selectedAgent.dealCount)} 次</span>
                  <span>{selectedAgent.estimatedDuration}</span>
                </div>
              </div>
            </div>

            <section className="detail-card">
              <div className="card-heading">
                <h3>交付说明</h3>
                <span>{formatMoney(selectedAgent.priceCents)} / 次</span>
              </div>
              <p className="muted-copy">{selectedAgent.deliverable}</p>
            </section>

            <section className="composer-card">
              <label htmlFor="requirement">任务需求</label>
              <textarea
                id="requirement"
                aria-label="任务需求"
                onChange={(event) => setRequirement(event.target.value)}
                value={requirement}
              />
              <div className="composer-actions">
                <span>将扣减 {selectedAgent.priceUses || 1} 次</span>
                <button className="primary-button" type="button" onClick={confirmCheckout}>
                  开始执行
                </button>
              </div>
            </section>
          </>
        ) : (
          <div className="empty-detail">从中间列表选择一个 Agent。</div>
        )}
      </section>
    );
  }

  return (
    <main className="split-shell">
      <aside className="rail" aria-label="主导航">
        <div className="app-mark">O</div>
        <nav>
          {sections.map((section) => (
            <button
              className={activeSection === section.id ? "is-active" : ""}
              key={section.id}
              type="button"
              onClick={() => setActiveSection(section.id)}
            >
              <NavIcon>{section.icon}</NavIcon>
              <span>{section.label}</span>
            </button>
          ))}
        </nav>
      </aside>

      <section className="list-pane" aria-label="列表区">
        {renderMiddlePane()}
      </section>

      <section className="content-pane" aria-label="详情区">
        {renderDetailPane()}
      </section>

      {toast && (
        <div className="toast" role="status">
          {toast}
        </div>
      )}
    </main>
  );
}
