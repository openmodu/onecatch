import { StatusBadge } from "../../ui/primitives.jsx";
import { statusLabel } from "../constants.js";

export default function StatusPill({ status, active }) {
  return <StatusBadge status={status || "ready"} className="status-pill">{active && <span className="pulse" />}{statusLabel[status] || status}</StatusBadge>;
}
