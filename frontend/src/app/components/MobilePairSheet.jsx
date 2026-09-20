import { useEffect, useState } from "react";
import { Check, Link2, LoaderCircle, Monitor, RefreshCw, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { MobileBinding } from "../../../bindings/github.com/openmodu/onecatch/internal/transport/wails/index.js";

const discoverWorkers = () => MobileBinding.DiscoverWorkers();

export default function PairSheet({ open, busy, initialURL = "https://", onClose, onPair, discover = discoverWorkers }) {
  const [baseURL, setBaseURL] = useState(initialURL || "https://");
  const [code, setCode] = useState("");
  const [manual, setManual] = useState(false);
  const [nearby, setNearby] = useState([]);
  const [searching, setSearching] = useState(false);
  const [searchFailed, setSearchFailed] = useState(false);
  const [scan, setScan] = useState(0);
  useEffect(() => {
    if (!open) return;
    setBaseURL(initialURL || "https://");
    setManual(Boolean(initialURL && initialURL !== "https://"));
    setCode("");
  }, [initialURL, open]);
  useEffect(() => {
    if (!open) return undefined;
    let current = true;
    setSearching(true);
    setSearchFailed(false);
    setNearby([]);
    Promise.resolve().then(discover).then((items) => {
      if (current) setNearby((items || []).filter((item) => item.addresses?.length));
    }).catch(() => {
      if (current) setSearchFailed(true);
    }).finally(() => {
      if (current) setSearching(false);
    });
    return () => { current = false; };
  }, [open, scan, discover]);
  if (!open) return null;
  const selected = nearby.find((item) => item.addresses.includes(baseURL));
  const validAddress = /^https?:\/\/[^\s/]+/i.test(baseURL.trim());
  return <div className="mobile-sheet-backdrop" role="presentation" onPointerDown={(event) => event.target === event.currentTarget && !busy && onClose()}>
    <section className="mobile-sheet mobile-pair-sheet" role="dialog" aria-modal="true" aria-labelledby="pair-title">
      <div className="mobile-sheet-handle" />
      <header><div><small>安全配对</small><h2 id="pair-title">连接电脑</h2></div><button type="button" className="mobile-icon-button" aria-label="关闭" disabled={busy} onClick={onClose}><X /></button></header>
      <p className="mobile-sheet-copy">电脑和手机连接同一局域网。在电脑的 OneCatch「设置 › 手机连接」中开启连接并生成配对码，然后在下方选择电脑。</p>
      <div className="mobile-discovery-heading"><strong>附近的电脑</strong><Button type="button" variant="ghost" size="sm" disabled={searching || busy} onClick={() => setScan((value) => value + 1)}><RefreshCw className={searching ? "animate-spin" : ""} />{searching ? "搜索中" : "重新搜索"}</Button></div>
      <div className="mobile-discovery-results" aria-busy={searching}>
        {searching ? <p role="status"><LoaderCircle className="animate-spin" size={16} />正在搜索局域网内的 OneCatch…</p>
          : nearby.length ? nearby.map((item) => <button type="button" className="mobile-discovered-worker" key={item.id} aria-pressed={selected?.id === item.id} disabled={busy} onClick={() => { setBaseURL(item.addresses[0]); setManual(false); }}>
            <Monitor /><span><strong>{item.name}</strong><small>{item.addresses[0]}</small></span>{selected?.id === item.id && <Check />}
          </button>)
            : <p role="status">{searchFailed ? "搜索暂不可用。请检查本地网络权限，或手动输入电脑地址。" : "未发现电脑。请确认电脑已开启「手机连接」、两端在同一局域网，并已允许本地网络访问。"}</p>}
      </div>
      <form onSubmit={(event) => { event.preventDefault(); if (validAddress) void onPair({ baseURL, code }).then((ok) => { if (ok) { setCode(""); onClose(); } }); }}>
        <button type="button" className="mobile-pair-manual-toggle" aria-expanded={manual} disabled={busy} onClick={() => setManual((value) => !value)}>{manual ? "收起手动地址" : "手动输入地址"}</button>
        {manual && <label><span>Worker 地址</span><Input value={baseURL} disabled={busy} inputMode="url" autoCapitalize="none" autoCorrect="off" placeholder="https://192.168.1.20:9232" onChange={(event) => setBaseURL(event.target.value)} /></label>}
        <label><span>一次性配对码</span><Input value={code} disabled={busy} autoCapitalize="characters" autoCorrect="off" maxLength={16} placeholder="例如 ABCD-EFGH" onChange={(event) => setCode(event.target.value.toUpperCase())} /></label>
        <Button className="mobile-main-action" type="submit" disabled={busy || !validAddress || !code.trim()}>{busy ? <><LoaderCircle className="animate-spin" />正在连接</> : <><Link2 />{selected ? `连接 ${selected.name}` : "连接电脑"}</>}</Button>
      </form>
      <p className="mobile-security-note">配对码 10 分钟内有效且只能用一次。发现电脑后仍需输入配对码；配对后 IP 变化会自动重连。独立 Worker 可执行 <code>onecatch worker --pair</code> 生成配对码。</p>
    </section>
  </div>;
}
