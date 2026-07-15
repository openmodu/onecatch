import { useTranslation } from "react-i18next";
import { StatusBadge } from "../../ui/primitives.jsx";
import { statusKey } from "../constants.js";

export default function StatusPill({ status, active }) {
  const { t } = useTranslation();
  return <StatusBadge status={status || "ready"} className="status-pill">{active && <span className="pulse" />}{t(statusKey(status), { defaultValue: status || t("status.ready") })}</StatusBadge>;
}
