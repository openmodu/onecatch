import { ChevronDown } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { runtimeHarness, runtimeHarnessOptions, selectRuntimeHarness } from "../runtimeHarnesses.js";

export default function HarnessSelector({ value, onChange, runtimes = [], readOnly = false, className = "" }) {
  const { t } = useTranslation();
  const harness = runtimeHarness(value?.harness);

  if (readOnly) {
    return <span className={`harness-profile-read-only ${className}`.trim()} aria-label={`${t("task.harness")}: ${harness.label}`} title={`${t("task.harness")}: ${harness.label}`}>{harness.label}</span>;
  }

  const options = runtimeHarnessOptions(runtimes, t("task.harnessUnavailable"));
  return <DropdownMenu>
    <DropdownMenuTrigger asChild>
      <Button type="button" variant="ghost" className={`new-task-select harness ${className}`.trim()} aria-label={t("task.harness")} title={t("task.harness")}>
        <span>{harness.label}</span><ChevronDown size={14} aria-hidden="true" />
      </Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent className="harness-select-menu" side="top" align="start" sideOffset={8}>
      <DropdownMenuRadioGroup value={harness.id} onValueChange={(nextHarness) => onChange?.((current) => selectRuntimeHarness(current, nextHarness))}>
        {options.map((option) => <DropdownMenuRadioItem className="harness-select-option" value={option.value} disabled={option.disabled} key={option.value}>
          <span>{option.label}</span>{option.meta && <small>{option.meta}</small>}
        </DropdownMenuRadioItem>)}
      </DropdownMenuRadioGroup>
    </DropdownMenuContent>
  </DropdownMenu>;
}
