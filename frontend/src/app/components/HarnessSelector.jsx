import { Bot, ChevronDown } from "lucide-react";
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

export default function HarnessSelector({ value, onChange, runtimes = [], readOnly = false, agentLabel = false, className = "" }) {
  const { t } = useTranslation();
  const harness = runtimeHarness(value?.harness);
  const label = harness.label;
  const controlLabel = agentLabel ? t("task.executionTarget") : t("task.harness");
  const controlClass = agentLabel ? "new-task-select executor" : "new-task-select harness";

  if (readOnly) {
    return <span className={`${controlClass} is-read-only harness-profile-read-only ${className}`.trim()} aria-label={`${controlLabel}: ${harness.label}`} title={`${controlLabel}: ${harness.label}`}>{agentLabel && <Bot size={14} aria-hidden="true" />}<span>{label}</span></span>;
  }

  const options = runtimeHarnessOptions(runtimes, t("task.harnessUnavailable"));
  return <DropdownMenu>
    <DropdownMenuTrigger asChild>
      <Button type="button" variant="ghost" className={`${controlClass} ${className}`.trim()} aria-label={controlLabel} title={controlLabel}>
        {agentLabel && <Bot size={14} aria-hidden="true" />}<span>{label}</span><ChevronDown size={14} aria-hidden="true" />
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
