import { useTranslation } from "react-i18next";
import { taskCategory } from "../taskCategory.js";

export default function TaskCategoryBadge({ task }) {
  const { t } = useTranslation();
  if (!task) return null;
  const category = taskCategory(task);
  const description = t(`taskCategory.${category}`);
  const source = t(task.category ? (task.categorySource === "ai" ? "taskCategory.ai" : "taskCategory.manual") : "taskCategory.auto");
  return <span className="session-category-badge" data-category={category} title={`${description} · ${source}`} aria-label={`${description} · ${source}`}>{category === "other" ? t("taskCategory.other") : category}</span>;
}
