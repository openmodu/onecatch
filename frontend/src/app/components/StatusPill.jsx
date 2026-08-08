import { useTranslation } from "react-i18next";
import { StatusBadge } from "../../ui/primitives.jsx";
import { statusKey } from "../constants.js";

export default function StatusPill({ status, active }) {
  const { t } = useTranslation();
  return (
    <StatusBadge status={status || "ready"}>
      {active && <span className="size-1.5 shrink-0 animate-pulse rounded-full bg-current" />}
      {t(statusKey(status), { defaultValue: status || t("status.ready") })}
    </StatusBadge>
  );
}
