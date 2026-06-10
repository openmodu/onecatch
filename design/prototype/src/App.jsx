import { useMemo, useState } from "react";
import {
  Bell,
  Books,
  Briefcase,
  ChartBar,
  CheckCircle,
  Code,
  Coins,
  DownloadSimple,
  FilePdf,
  Gauge,
  GoogleLogo,
  House,
  ListChecks,
  Megaphone,
  Palette,
  Receipt,
  ShareFat,
  ShieldCheck,
  ShoppingCart,
  Sparkle,
  SignOut,
  Target,
  TrendUp,
  Wallet,
  WechatLogo,
} from "@phosphor-icons/react";
import reportPreview from "./assets/report-preview.png";

const categories = [
  { id: "all", label: "全部 Agent", icon: Gauge },
  { id: "content", label: "内容创作", icon: Sparkle },
  { id: "data", label: "数据分析", icon: ChartBar },
  { id: "marketing", label: "市场营销", icon: Megaphone },
  { id: "design", label: "设计创意", icon: Palette },
  { id: "dev", label: "开发与技术", icon: Code },
  { id: "office", label: "办公效率", icon: Briefcase },
  { id: "research", label: "行业研究", icon: Books },
];

const agents = [
  {
    id: "research",
    category: "research",
    name: "行业研究分析师",
    tag: "专家",
    icon: TrendUp,
    price: 19.9,
    rating: "4.9",
    uses: "1,268",
    eta: "2-4 小时",
    summary: "深度行业研究与竞品洞察，输出专业分析报告。",
    deliverable: "市场规模、竞争格局、趋势洞察、机会清单",
  },
  {
    id: "copy",
    category: "content",
    name: "内容增长写手",
    tag: "热门",
    icon: Sparkle,
    price: 8.8,
    rating: "4.8",
    uses: "3,420",
    eta: "30-60 分钟",
    summary: "生成公众号、朋友圈、短视频脚本与产品文案。",
    deliverable: "3 个标题方向、正文、分发建议",
  },
  {
    id: "data",
    category: "data",
    name: "经营数据分析师",
    tag: "企业",
    icon: ChartBar,
    price: 15.6,
    rating: "4.7",
    uses: "986",
    eta: "1-2 小时",
    summary: "处理表格数据，定位异常波动并输出经营建议。",
    deliverable: "指标拆解、异常解释、改进建议",
  },
  {
    id: "launch",
    category: "marketing",
    name: "新品上市策划",
    tag: "增长",
    icon: Target,
    price: 12.9,
    rating: "4.8",
    uses: "2,104",
    eta: "1 小时",
    summary: "为新品制定目标客群、渠道节奏与首轮推广方案。",
    deliverable: "人群画像、卖点主张、投放节奏",
  },
];

const initialOrders = [
  {
    id: "ORD20240524001029",
    title: "本次任务",
    agent: "行业研究分析师",
    status: "running",
    statusLabel: "执行中",
    date: "2024-05-24 10:30",
    eta: "2024-05-24 12:30",
    debit: 1,
    amount: 19.9,
  },
  {
    id: "ORD20240523000988",
    title: "竞品分析报告",
    agent: "经营数据分析师",
    status: "done",
    statusLabel: "已交付",
    date: "2024-05-23 16:24",
    eta: "2024-05-23 17:36",
    debit: 1,
    amount: 15.6,
  },
  {
    id: "ORD20240522000877",
    title: "市场规模研究",
    agent: "行业研究分析师",
    status: "done",
    statusLabel: "已交付",
    date: "2024-05-22 09:12",
    eta: "2024-05-22 11:08",
    debit: 1,
    amount: 19.9,
  },
  {
    id: "ORD20240521000766",
    title: "行业趋势洞察",
    agent: "行业研究分析师",
    status: "pending",
    statusLabel: "待支付",
    date: "2024-05-21 14:18",
    eta: "待确认",
    debit: 0,
    amount: 19.9,
  },
];

const tabs = ["Agent 详情", "需求填写", "扣次确认", "执行中", "交付物"];

const statusFilters = [
  { id: "all", label: "全部订单" },
  { id: "pending", label: "待支付" },
  { id: "running", label: "执行中" },
  { id: "done", label: "已交付" },
];

function formatMoney(value) {
  return `¥ ${value.toFixed(2)}`;
}

export function App() {
  const [loggedIn, setLoggedIn] = useState(false);
  const [authProvider, setAuthProvider] = useState("wechat");
  const [activeCategory, setActiveCategory] = useState("all");
  const [selectedAgentId, setSelectedAgentId] = useState("research");
  const [selectedTab, setSelectedTab] = useState("扣次确认");
  const [credits, setCredits] = useState(12);
  const [requirement, setRequirement] = useState(
    "请帮我完成 2024 年中国 AI 研报市场的研究分析，包括市场规模、竞争格局、主要玩家、趋势洞察和机会点，生成一份完整的研究报告。",
  );
  const [orders, setOrders] = useState(initialOrders);
  const [orderFilter, setOrderFilter] = useState("all");
  const [selectedOrderId, setSelectedOrderId] = useState(initialOrders[0].id);
  const [paymentOpen, setPaymentOpen] = useState(false);
  const [rightRailOpen, setRightRailOpen] = useState(true);
  const [toast, setToast] = useState("");
  const [buyCount, setBuyCount] = useState(10);

  const visibleAgents = useMemo(() => {
    if (activeCategory === "all") return agents;
    return agents.filter((agent) => agent.category === activeCategory);
  }, [activeCategory]);

  const selectedAgent =
    agents.find((agent) => agent.id === selectedAgentId) ?? agents[0];
  const selectedOrder =
    orders.find((order) => order.id === selectedOrderId) ?? orders[0];
  const authLabel = authProvider === "google" ? "Google 邮箱用户" : "微信用户";
  const AuthIcon = authProvider === "google" ? GoogleLogo : WechatLogo;

  function showToast(message) {
    setToast(message);
    window.clearTimeout(showToast.timer);
    showToast.timer = window.setTimeout(() => setToast(""), 2200);
  }

  function chooseAgent(agentId) {
    setSelectedAgentId(agentId);
    setSelectedTab("Agent 详情");
  }

  function confirmUse() {
    if (!loggedIn) {
      showToast("请先登录后使用 Agent");
      return;
    }
    if (credits <= 0) {
      setPaymentOpen(true);
      return;
    }
    setCredits((value) => value - 1);
    const createdAt = "2024-05-24 10:31";
    const newOrder = {
      id: `ORD${Date.now().toString().slice(-12)}`,
      title: "新建任务",
      agent: selectedAgent.name,
      status: "running",
      statusLabel: "执行中",
      date: createdAt,
      eta: "预计 2-4 小时",
      debit: 1,
      amount: selectedAgent.price,
    };
    setOrders((items) => [newOrder, ...items]);
    setSelectedOrderId(newOrder.id);
    setSelectedTab("执行中");
    showToast("已扣减 1 次，Agent 开始执行");
  }

  function buyCredits() {
    setCredits((value) => value + buyCount);
    setPaymentOpen(false);
    showToast(`购买成功，已增加 ${buyCount} 次`);
  }

  function completeSelectedOrder() {
    setOrders((items) =>
      items.map((order) =>
        order.id === selectedOrder.id
          ? { ...order, status: "done", statusLabel: "已交付" }
          : order,
      ),
    );
    setSelectedTab("交付物");
    showToast("订单已更新为已交付");
  }

  function loginWith(provider) {
    setLoggedIn(true);
    setAuthProvider(provider);
    showToast(provider === "google" ? "已通过 Google 邮箱登录" : "已通过微信授权登录");
  }

  function logout() {
    setLoggedIn(false);
    showToast("已退出登录");
  }

  return (
    <main className="app-shell">
      <aside className="sidebar">
        <div className="brand">
          <strong>Agent 服务市场</strong>
          <span>专业 Agent · 按使用次数计费</span>
        </div>

        <nav className="nav-block" aria-label="主导航">
          <button className="nav-item active" type="button">
            <House size={19} weight="duotone" />
            工作台
          </button>
        </nav>

        <nav className="nav-block" aria-label="Agent 市场">
          <p>Agent 市场</p>
          {categories.map((category) => {
            const Icon = category.icon;
            return (
              <button
                className={`nav-item ${activeCategory === category.id ? "active-soft" : ""}`}
                key={category.id}
                type="button"
                onClick={() => setActiveCategory(category.id)}
              >
                <Icon size={19} weight="duotone" />
                {category.label}
              </button>
            );
          })}
        </nav>

        <nav className="nav-block" aria-label="次数与订单">
          <p>次数与用量</p>
          <button className="nav-item active-soft" type="button">
            <Coins size={19} weight="duotone" />
            用量总览
          </button>
          <button className="nav-item" type="button" onClick={() => setPaymentOpen(true)}>
            <Wallet size={19} weight="duotone" />
            购买次数
          </button>
          <button className="nav-item" type="button">
            <Receipt size={19} weight="duotone" />
            使用记录
          </button>
        </nav>

        <nav className="nav-block" aria-label="我的订单">
          <p>我的订单</p>
          {statusFilters.map((filter) => (
            <button
              className={`nav-item ${orderFilter === filter.id ? "active-soft" : ""}`}
              key={filter.id}
              type="button"
              onClick={() => setOrderFilter(filter.id)}
            >
              <ListChecks size={19} weight="duotone" />
              {filter.label}
            </button>
          ))}
        </nav>

        <div className="sidebar-footer">
          <section className="auth-card" aria-label="账号登录">
            {loggedIn ? (
              <>
                <div className="auth-user">
                  <span className="auth-avatar">
                    <AuthIcon size={20} weight="fill" />
                  </span>
                  <span>
                    <strong>{authLabel}</strong>
                    <small>{authProvider === "google" ? "邮箱授权已连接" : "微信授权已连接"}</small>
                  </span>
                </div>
                <button className="auth-logout" type="button" onClick={logout}>
                  <SignOut size={16} />
                  退出登录
                </button>
              </>
            ) : (
              <>
                <p>登录账号</p>
                <button className="auth-button wechat" type="button" onClick={() => loginWith("wechat")}>
                  <WechatLogo size={18} weight="fill" />
                  微信授权登录
                </button>
                <button className="auth-button google" type="button" onClick={() => loginWith("google")}>
                  <GoogleLogo size={18} weight="bold" />
                  Google 邮箱登录
                </button>
              </>
            )}
          </section>

          <button className="help-link" type="button" onClick={() => showToast("帮助中心暂未接入")}>
            <ShieldCheck size={18} weight="duotone" />
            帮助中心
          </button>
        </div>
      </aside>

      <section className="workspace">
        <header className="topbar">
          <button className="back-link" type="button" onClick={() => setSelectedTab("Agent 详情")}>
            返回工作台
          </button>
          <div className="top-actions">
            <button className="ghost-link" type="button">帮助中心</button>
            <button className="icon-button" type="button" aria-label="通知">
              <Bell size={18} />
            </button>
          </div>
        </header>

        <div className={`content-grid ${rightRailOpen ? "" : "is-rail-collapsed"}`}>
          <section className="main-column">
            <div className="agent-hero">
              <div className="agent-mark">
                <selectedAgent.icon size={46} weight="duotone" />
              </div>
              <div className="agent-copy">
                <div className="title-line">
                  <h1>{selectedAgent.name}</h1>
                  <span>{selectedAgent.tag}</span>
                </div>
                <p>{selectedAgent.summary}</p>
                <div className="meta-row">
                  <strong>★ {selectedAgent.rating}</strong>
                  <span>成交 {selectedAgent.uses} 次</span>
                  <span>平均交付 {selectedAgent.eta}</span>
                </div>
              </div>
              <div className="price-box">
                <span>单次价格</span>
                <strong>{formatMoney(selectedAgent.price)}</strong>
                <small>/ 次</small>
              </div>
            </div>

            <div className="agent-strip">
              {visibleAgents.map((agent) => {
                const Icon = agent.icon;
                return (
                  <button
                    className={`agent-chip ${selectedAgentId === agent.id ? "selected" : ""}`}
                    key={agent.id}
                    type="button"
                    onClick={() => chooseAgent(agent.id)}
                  >
                    <Icon size={17} weight="duotone" />
                    <span>{agent.name}</span>
                    <strong>{formatMoney(agent.price)}</strong>
                  </button>
                );
              })}
            </div>

            <div className="tabs" role="tablist" aria-label="订单流程">
              {tabs.map((tab, index) => (
                <button
                  className={selectedTab === tab ? "selected" : ""}
                  key={tab}
                  type="button"
                  onClick={() => setSelectedTab(tab)}
                >
                  <span>{index + 1}</span>
                  {tab}
                </button>
              ))}
            </div>

            <div className="brief-grid">
              <section className="panel">
                <div className="panel-title">
                  <h2>{selectedTab === "需求填写" ? "编辑需求" : "需求摘要"}</h2>
                  <button type="button" onClick={() => setSelectedTab("需求填写")}>编辑需求</button>
                </div>
                <textarea
                  value={requirement}
                  onChange={(event) => setRequirement(event.target.value)}
                  readOnly={selectedTab !== "需求填写"}
                  aria-label="任务需求"
                />
              </section>

              <section className="panel checklist-panel">
                <h2>扣次说明</h2>
                <ul>
                  <li><CheckCircle size={18} weight="fill" /> 本次任务将扣减 1 次</li>
                  <li><CheckCircle size={18} weight="fill" /> 单次独立交付，仅针对本次需求</li>
                  <li><CheckCircle size={18} weight="fill" /> 开始执行后不可撤销</li>
                </ul>
                <button className="soft-button" type="button" onClick={() => setSelectedTab("执行中")}>
                  查看执行状态
                </button>
              </section>

              <section className="panel balance-panel">
                <h2>当前用量余额</h2>
                <div className="balance-number">
                  <strong>{credits}</strong>
                  <span>次</span>
                </div>
                <button className="outline-button" type="button" onClick={() => setPaymentOpen(true)}>
                  <ShoppingCart size={17} weight="duotone" />
                  购买次数
                </button>
              </section>
            </div>

            <section className="checkout-panel">
              <div className="calc-row">
                <span>本次扣减</span>
                <strong>1 次</strong>
              </div>
              <div className="calc-divider" />
              <div className="calc-row">
                <span>单次价格</span>
                <strong>{formatMoney(selectedAgent.price)}</strong>
              </div>
              <div className="calc-divider" />
              <div className="calc-row due">
                <span>应付金额</span>
                <strong>{formatMoney(selectedAgent.price)}</strong>
              </div>
              <label className="pay-select">
                支付方式
                <select>
                  <option>微信支付</option>
                  <option>余额扣次</option>
                </select>
              </label>
              <button className="primary-button" type="button" onClick={confirmUse}>
                确认并支付（本次扣减 1 次）
              </button>
            </section>

            <section className="progress-panel">
              <h2>执行进度</h2>
              {[
                ["done", "需求已提交", "2024-05-24 10:30", "您已提交需求，等待扣次确认"],
                ["current", "扣次确认中", "2024-05-24 10:31", "待支付以扣减 1 次并开始执行"],
                ["next", "执行中", "预计 2024-05-24 10:31 开始", "Agent 正在进行研究与分析"],
                ["next", "交付物生成中", "预计 2024-05-24 12:30 完成", "报告生成并校验中"],
                ["next", "已交付", "预计 2024-05-24 12:30 完成", "交付物已生成并可查看下载"],
              ].map(([state, title, time, desc]) => (
                <div className={`timeline-item ${state}`} key={title}>
                  <span className="timeline-dot" />
                  <strong>{title}</strong>
                  <time>{time}</time>
                  <p>{desc}</p>
                </div>
              ))}
            </section>
          </section>

          {rightRailOpen ? (
          <aside className="right-rail">
            <button
              className="rail-collapse-button"
              type="button"
              aria-expanded={rightRailOpen}
              onClick={() => setRightRailOpen(false)}
            >
              <Receipt size={17} weight="duotone" />
              收起右侧
            </button>
            <section className="rail-card usage-card">
              <div className="rail-title">
                <h2>用量与计费</h2>
                <button type="button" onClick={() => setPaymentOpen(true)}>购买次数</button>
              </div>
              <div className="usage-stats">
                <div>
                  <span>剩余次数</span>
                  <strong>{credits}</strong>
                  <small>次</small>
                </div>
                <div>
                  <span>单次价格</span>
                  <strong>{formatMoney(selectedAgent.price)}</strong>
                </div>
              </div>
            </section>

            <section className="rail-card">
              <div className="rail-title">
                <h2>使用记录</h2>
                <button type="button" onClick={() => setOrderFilter("all")}>查看全部</button>
              </div>
              <div className="usage-list">
                {orders.slice(0, 5).map((order) => (
                  <button
                    className={selectedOrderId === order.id ? "selected" : ""}
                    key={order.id}
                    type="button"
                    onClick={() => setSelectedOrderId(order.id)}
                  >
                    <time>{order.date.slice(0, 10)}</time>
                    <span>
                      <strong>{order.title}</strong>
                      <small>订单号：{order.id}</small>
                    </span>
                    <em>{order.debit ? `-${order.debit} 次` : "待扣次"}</em>
                  </button>
                ))}
              </div>
            </section>

            <section className="rail-card">
              <div className="rail-title">
                <h2>订单信息</h2>
                <button type="button" onClick={completeSelectedOrder}>标记交付</button>
              </div>
              <dl className="order-info">
                <div><dt>订单号</dt><dd>{selectedOrder.id}</dd></div>
                <div><dt>创建时间</dt><dd>{selectedOrder.date}</dd></div>
                <div><dt>订单状态</dt><dd className={selectedOrder.status}>{selectedOrder.statusLabel}</dd></div>
                <div><dt>预计完成</dt><dd>{selectedOrder.eta}</dd></div>
                <div><dt>本次扣减</dt><dd>{selectedOrder.debit || 1} 次</dd></div>
                <div><dt>总金额</dt><dd>{formatMoney(selectedOrder.amount)}</dd></div>
              </dl>
            </section>

            <section className="rail-card delivery-card">
              <div className="rail-title">
                <h2>交付物预览</h2>
                <button type="button" onClick={() => setSelectedTab("交付物")}>预览</button>
              </div>
              <div className="file-row">
                <FilePdf size={29} weight="fill" />
                <span>
                  <strong>2024年中国AI研报市场研究报告.pdf</strong>
                  <small>2.4 MB · PDF</small>
                </span>
              </div>
              <img src={reportPreview} alt="AI 行业研究报告预览" />
              <div className="delivery-actions">
                <button type="button" onClick={() => showToast("已模拟下载报告")}>
                  <DownloadSimple size={18} />
                  下载报告
                </button>
                <button type="button" onClick={() => showToast("分享链接已生成")}>
                  <ShareFat size={18} />
                  分享报告
                </button>
              </div>
            </section>
          </aside>
          ) : (
            <button
              className="rail-peek"
              type="button"
              aria-expanded={rightRailOpen}
              onClick={() => setRightRailOpen(true)}
            >
              <Receipt size={20} weight="duotone" />
              <span>详情</span>
              <small>{credits} 次</small>
            </button>
          )}
        </div>
      </section>

      {paymentOpen && (
        <div className="modal-backdrop" role="presentation" onClick={() => setPaymentOpen(false)}>
          <section className="payment-modal" role="dialog" aria-modal="true" onClick={(event) => event.stopPropagation()}>
            <div className="modal-title">
              <h2>购买使用次数</h2>
              <button type="button" onClick={() => setPaymentOpen(false)}>关闭</button>
            </div>
            <p>次数可用于任意 Agent。使用任务时每次扣减 1 次，未开始执行不会扣次。</p>
            <div className="count-options">
              {[5, 10, 20].map((count) => (
                <button
                  className={buyCount === count ? "selected" : ""}
                  key={count}
                  type="button"
                  onClick={() => setBuyCount(count)}
                >
                  <strong>{count} 次</strong>
                  <span>{formatMoney(count * selectedAgent.price)}</span>
                </button>
              ))}
            </div>
            <div className="qr-block">
              <WechatLogo size={42} weight="fill" />
              <span>微信支付模拟</span>
              <small>点击确认后立即增加次数</small>
            </div>
            <button className="primary-button" type="button" onClick={buyCredits}>
              确认支付 {formatMoney(buyCount * selectedAgent.price)}
            </button>
          </section>
        </div>
      )}

      {toast && <div className="toast">{toast}</div>}
    </main>
  );
}
