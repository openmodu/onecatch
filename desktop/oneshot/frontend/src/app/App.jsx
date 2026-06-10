import { useEffect, useMemo, useState } from "react";
import {
  AgentBinding,
  AuthBinding,
} from "../../bindings/github.com/openmodu/oneshot/desktop/oneshot/bindings/index.js";

const categories = [
  { id: "all", label: "全部 Agent", icon: "A" },
  { id: "content", label: "内容创作", icon: "C" },
  { id: "data", label: "数据分析", icon: "D" },
  { id: "marketing", label: "市场营销", icon: "M" },
  { id: "design", label: "设计创意", icon: "P" },
  { id: "dev", label: "开发与技术", icon: "T" },
  { id: "office", label: "办公效率", icon: "O" },
  { id: "research", label: "行业研究", icon: "R" },
];

const orders = [
  {
    id: "ORD20260610001029",
    title: "本次任务",
    agent: "行业研究分析师",
    status: "running",
    statusLabel: "执行中",
    createdAt: "2026-06-10 10:30",
    eta: "2026-06-10 12:30",
    debit: 1,
    amount: 1990,
  },
  {
    id: "ORD20260609000988",
    title: "竞品分析报告",
    agent: "经营数据分析师",
    status: "delivered",
    statusLabel: "已交付",
    createdAt: "2026-06-09 16:24",
    eta: "2026-06-09 17:36",
    debit: 1,
    amount: 1560,
  },
  {
    id: "ORD20260608000877",
    title: "市场规模研究",
    agent: "行业研究分析师",
    status: "pending",
    statusLabel: "待支付",
    createdAt: "2026-06-08 09:12",
    eta: "待确认",
    debit: 0,
    amount: 1990,
  },
];

const orderFilters = [
  { id: "all", label: "全部订单" },
  { id: "pending", label: "待支付" },
  { id: "running", label: "执行中" },
  { id: "delivered", label: "已交付" },
];

const steps = ["Agent 详情", "需求填写", "扣次确认", "执行中", "交付物"];

function formatMoney(cents) {
  return `¥ ${(Number(cents || 0) / 100).toFixed(2)}`;
}

function formatDealCount(value) {
  return Number(value || 0).toLocaleString("zh-CN");
}

function NavIcon({ children }) {
  return (
    <span className="nav-icon" aria-hidden="true">
      {children}
    </span>
  );
}

export default function App() {
  const [agents, setAgents] = useState([]);
  const [agentStatus, setAgentStatus] = useState("loading");
  const [currentUser, setCurrentUser] = useState(null);
  const [authProvider, setAuthProvider] = useState("");
  const [authStatus, setAuthStatus] = useState("checking");
  const [toast, setToast] = useState("");
  const [activeCategory, setActiveCategory] = useState("all");
  const [selectedAgentId, setSelectedAgentId] = useState("");
  const [selectedStep, setSelectedStep] = useState("扣次确认");
  const [selectedOrderId, setSelectedOrderId] = useState(orders[0].id);
  const [orderFilter, setOrderFilter] = useState("all");
  const [inspectorOpen, setInspectorOpen] = useState(true);
  const [requirement, setRequirement] = useState(
    "请帮我完成 2026 年中国 AI Agent 服务市场研究，包括市场规模、主要玩家、收费模式、增长机会和进入建议。",
  );

  const visibleAgents = useMemo(() => {
    if (activeCategory === "all") return agents;
    return agents.filter((agent) => agent.category === activeCategory);
  }, [activeCategory, agents]);

  const visibleOrders = useMemo(() => {
    if (orderFilter === "all") return orders;
    return orders.filter((order) => order.status === orderFilter);
  }, [orderFilter]);

  const selectedAgent =
    visibleAgents.find((agent) => agent.id === selectedAgentId) ??
    visibleAgents[0] ??
    null;
  const selectedOrder =
    orders.find((order) => order.id === selectedOrderId) ?? orders[0];
  const authProviderLabel = authProvider === "google" ? "Google 邮箱" : "微信";

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

  function selectCategory(categoryId) {
    setActiveCategory(categoryId);
    const nextAgent =
      categoryId === "all"
        ? agents[0]
        : agents.find((agent) => agent.category === categoryId);
    setSelectedAgentId(nextAgent?.id || "");
  }

  function showToast(message) {
    setToast(message);
    window.clearTimeout(showToast.timer);
    showToast.timer = window.setTimeout(() => setToast(""), 2200);
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

  function confirmCheckout() {
    if (!selectedAgent) {
      showToast("请先选择可用 Agent");
      return;
    }
    if (!currentUser) {
      showToast("请先登录后再创建订单");
      return;
    }
    setSelectedStep("执行中");
    showToast("已确认扣次，订单开始执行");
  }

  return (
    <main className="workbench-shell">
      <aside className="sidebar" aria-label="工作台导航">
        <header className="brand">
          <strong>Oneshot</strong>
          <span>Agent 服务市场</span>
        </header>

        <nav className="nav-section" aria-label="主导航">
          <button className="nav-item is-active" type="button">
            <NavIcon>H</NavIcon>
            工作台
          </button>
        </nav>

        <nav className="nav-section" aria-label="Agent 分类">
          <p>Agent 市场</p>
          {categories.map((category) => (
            <button
              className={`nav-item ${
                activeCategory === category.id ? "is-selected" : ""
              }`}
              key={category.id}
              type="button"
              onClick={() => selectCategory(category.id)}
            >
              <NavIcon>{category.icon}</NavIcon>
              {category.label}
            </button>
          ))}
        </nav>

        <nav className="nav-section" aria-label="次数与订单">
          <p>次数与用量</p>
          <button className="nav-item is-selected" type="button">
            <NavIcon>U</NavIcon>
            用量总览
          </button>
          <button className="nav-item" type="button">
            <NavIcon>B</NavIcon>
            购买次数
          </button>
          <button className="nav-item" type="button">
            <NavIcon>L</NavIcon>
            使用记录
          </button>
        </nav>

        <nav className="nav-section" aria-label="订单筛选">
          <p>我的订单</p>
          {orderFilters.map((filter) => (
            <button
              className={`nav-item ${
                orderFilter === filter.id ? "is-selected" : ""
              }`}
              key={filter.id}
              type="button"
              onClick={() => setOrderFilter(filter.id)}
            >
              <NavIcon>O</NavIcon>
              {filter.label}
            </button>
          ))}
        </nav>

        <section className="account-panel" aria-label="账号区">
          {currentUser ? (
            <>
              <p>已登录</p>
              <div className="account-user">
                <strong>{currentUser.displayName || currentUser.email}</strong>
                <small>{authProviderLabel}授权已连接</small>
              </div>
              <button className="account-button secondary" type="button" onClick={logout}>
                退出登录
              </button>
            </>
          ) : (
            <>
              <p>{authStatus === "checking" ? "正在检查账号" : "登录账号"}</p>
              <button
                className="account-button"
                type="button"
                disabled={authStatus === "checking"}
                onClick={() => login("wechat")}
              >
                微信授权登录
              </button>
              <button
                className="account-button secondary"
                type="button"
                disabled={authStatus === "checking"}
                onClick={() => login("google")}
              >
                Google 邮箱登录
              </button>
            </>
          )}
        </section>
      </aside>

      <section className="workspace">
        <header className="topbar">
          <button className="text-button" type="button">
            返回工作台
          </button>
          <div className="topbar-actions">
            <button className="text-button" type="button">
              帮助中心
            </button>
            <button className="round-button" type="button" aria-label="通知">
              N
            </button>
          </div>
        </header>

        <div
          className={`workspace-grid ${
            inspectorOpen ? "" : "is-inspector-collapsed"
          }`}
        >
          <section className="main-pane" aria-label="Agent 工作区">
            <section className="agent-summary">
              <div className="agent-avatar" aria-hidden="true">
                {selectedAgent?.name?.slice(0, 1) || "A"}
              </div>
              <div className="agent-heading">
                <div className="title-row">
                  <h1>{selectedAgent?.name || "暂无 Agent"}</h1>
                  <span>{selectedAgent?.tags?.[0] || "待补充"}</span>
                </div>
                <p>
                  {selectedAgent?.description ||
                    (agentStatus === "loading" ? "正在加载 Agent 目录" : "该分类暂无可用 Agent")}
                </p>
                <div className="metric-row">
                  <strong>评分 {selectedAgent?.rating || "-"}</strong>
                  <span>成交 {formatDealCount(selectedAgent?.dealCount)} 次</span>
                  <span>平均交付 {selectedAgent?.estimatedDuration || "待确认"}</span>
                </div>
              </div>
              <div className="price-panel">
                <span>单次价格</span>
                <strong>{formatMoney(selectedAgent?.priceCents)}</strong>
                <small>/ 次</small>
              </div>
            </section>

            <section className="agent-strip" aria-label="可选 Agent">
              {visibleAgents.length > 0 ? (
                visibleAgents.map((agent) => (
                  <button
                    className={`agent-chip ${
                      selectedAgentId === agent.id ? "is-selected" : ""
                    }`}
                    key={agent.id}
                    type="button"
                    onClick={() => {
                      setSelectedAgentId(agent.id);
                      setSelectedStep("Agent 详情");
                    }}
                  >
                    <span>{agent.name}</span>
                    <strong>{formatMoney(agent.priceCents)}</strong>
                  </button>
                ))
              ) : (
                <div className="empty-strip">
                  {agentStatus === "loading" ? "正在加载 Agent" : "该分类暂无 Agent"}
                </div>
              )}
            </section>

            <nav className="step-tabs" aria-label="任务流程">
              {steps.map((step, index) => (
                <button
                  className={selectedStep === step ? "is-selected" : ""}
                  key={step}
                  type="button"
                  onClick={() => setSelectedStep(step)}
                >
                  <span>{index + 1}</span>
                  {step}
                </button>
              ))}
            </nav>

            <div className="detail-grid">
              <section className="panel requirement-panel">
                <div className="panel-title">
                  <h2>{selectedStep === "需求填写" ? "编辑需求" : "需求摘要"}</h2>
                  <button type="button" onClick={() => setSelectedStep("需求填写")}>
                    编辑需求
                  </button>
                </div>
                <textarea
                  aria-label="任务需求"
                  onChange={(event) => setRequirement(event.target.value)}
                  readOnly={selectedStep !== "需求填写"}
                  value={requirement}
                />
              </section>

              <section className="panel">
                <h2>交付物说明</h2>
                <ul className="check-list">
                  <li>本次执行将扣减 1 次</li>
                  <li>{selectedAgent?.deliverable || "请选择一个可用 Agent"}</li>
                  <li>开始执行后进入订单进度流转</li>
                </ul>
              </section>

              <section className="panel balance-panel">
                <h2>当前余额</h2>
                <div className="balance-number">
                  <strong>12</strong>
                  <span>次</span>
                </div>
                <button className="outline-button" type="button">
                  购买次数
                </button>
              </section>
            </div>

            <section className="checkout-bar" aria-label="扣次确认">
              <div>
                <span>本次扣减</span>
                <strong>1 次</strong>
              </div>
              <div>
                <span>单次价格</span>
                <strong>{formatMoney(selectedAgent?.priceCents)}</strong>
              </div>
              <div>
                <span>应付金额</span>
                <strong>{formatMoney(selectedAgent?.priceCents)}</strong>
              </div>
              <button className="primary-button" type="button" onClick={confirmCheckout}>
                确认并支付
              </button>
            </section>

            <section className="timeline-panel">
              <h2>执行进度</h2>
              {[
                ["done", "需求已提交", "2026-06-10 10:30"],
                ["current", "扣次确认中", "等待确认并扣减 1 次"],
                ["next", "执行中", `预计 ${selectedAgent?.estimatedDuration || "待确认"}`],
                ["next", "交付物生成中", "报告生成并校验中"],
                ["next", "已交付", "交付物可查看、下载和分享"],
              ].map(([state, title, time]) => (
                <div className={`timeline-row ${state}`} key={title}>
                  <span className="timeline-dot" />
                  <strong>{title}</strong>
                  <time>{time}</time>
                </div>
              ))}
            </section>
          </section>

          {inspectorOpen ? (
            <aside className="inspector" aria-label="右侧 Inspector">
              <button
                className="inspector-toggle"
                type="button"
                aria-expanded="true"
                onClick={() => setInspectorOpen(false)}
              >
                收起 Inspector
              </button>

              <section className="inspector-card usage-card">
                <div className="card-title">
                  <h2>用量与计费</h2>
                  <button type="button">购买次数</button>
                </div>
                <div className="usage-grid">
                  <div>
                    <span>剩余次数</span>
                    <strong>12</strong>
                    <small>次</small>
                  </div>
                  <div>
                    <span>本次价格</span>
                    <strong>{formatMoney(selectedAgent?.priceCents)}</strong>
                  </div>
                </div>
              </section>

              <section className="inspector-card">
                <div className="card-title">
                  <h2>使用记录</h2>
                  <button type="button">查看全部</button>
                </div>
                <div className="record-list">
                  {visibleOrders.map((order) => (
                    <button
                      className={selectedOrderId === order.id ? "is-selected" : ""}
                      key={order.id}
                      type="button"
                      onClick={() => setSelectedOrderId(order.id)}
                    >
                      <span>
                        <strong>{order.title}</strong>
                        <small>{order.id}</small>
                      </span>
                      <em>{order.debit ? `-${order.debit} 次` : "待扣次"}</em>
                    </button>
                  ))}
                </div>
              </section>

              <section className="inspector-card">
                <div className="card-title">
                  <h2>订单信息</h2>
                  <button type="button">详情</button>
                </div>
                <dl className="order-info">
                  <div>
                    <dt>订单号</dt>
                    <dd>{selectedOrder.id}</dd>
                  </div>
                  <div>
                    <dt>关联 Agent</dt>
                    <dd>{selectedOrder.agent}</dd>
                  </div>
                  <div>
                    <dt>订单状态</dt>
                    <dd>{selectedOrder.statusLabel}</dd>
                  </div>
                  <div>
                    <dt>预计完成</dt>
                    <dd>{selectedOrder.eta}</dd>
                  </div>
                  <div>
                    <dt>总金额</dt>
                    <dd>{formatMoney(selectedOrder.amount)}</dd>
                  </div>
                </dl>
              </section>

              <section className="inspector-card delivery-card">
                <div className="card-title">
                  <h2>交付物预览</h2>
                  <button type="button">预览</button>
                </div>
                <div className="file-preview">
                  <strong>AI Agent 服务市场研究报告.pdf</strong>
                  <small>2.4 MB · PDF</small>
                </div>
              </section>
            </aside>
          ) : (
            <button
              className="inspector-peek"
              type="button"
              aria-expanded="false"
              onClick={() => setInspectorOpen(true)}
            >
              <span>Inspector</span>
              <small>12 次</small>
            </button>
          )}
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
