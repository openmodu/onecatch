import { useId, useMemo } from "react";

import { Button } from "@/components/ui/button";
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { cn } from "@/lib/utils";

const BUTTON_VARIANT = {
  primary: "default",
  accent: "secondary",
  muted: "outline",
  danger: "outline",
  cyan: "outline",
};

const BUTTON_TONE = {
  danger: "border-destructive/40 text-destructive hover:bg-destructive hover:text-destructive-foreground",
  cyan: "border-info/40 text-info hover:bg-info hover:text-info-foreground",
};

export function SettingsButton({ tone = "accent", compact = false, className = "", type = "button", ...props }) {
  return (
    <Button
      type={type}
      variant={BUTTON_VARIANT[tone] || "secondary"}
      size={compact ? "xs" : "sm"}
      className={cn("rounded-lg", BUTTON_TONE[tone], className)}
      {...props}
    />
  );
}

export function SettingsKicker({ children, className = "" }) {
  return <span className={cn("text-[11px] font-semibold uppercase tracking-[0.08em] text-muted-foreground", className)}>{children}</span>;
}

export function SettingsSection({ title, description, aside, children, className = "", contentClassName = "" }) {
  return (
    <section className={cn("mb-6", className)}>
      <div className="flex items-start justify-between gap-6 px-0.5 pb-3.5">
        <div className="min-w-0">
          <h3 className="m-0 text-[15px] font-semibold leading-tight text-foreground">{title}</h3>
          {description && <p className="mt-1.5 mb-0 max-w-3xl text-xs leading-relaxed text-muted-foreground">{description}</p>}
        </div>
        {aside}
      </div>
      <Card className="gap-0 overflow-hidden border-border/80 bg-card/72 py-0 shadow-[0_8px_26px_color-mix(in_oklab,var(--foreground)_4%,transparent)] backdrop-blur-sm">
        <CardContent className={cn("px-0", contentClassName)}>{children}</CardContent>
      </Card>
    </section>
  );
}

export function RuntimeSettingsCard({ title, description, aside, children, className = "" }) {
  return (
    <Card className={cn("mx-4 mb-3 gap-3 border-0 bg-muted/55 py-4 shadow-none last:mb-4", className)}>
      <CardHeader className="gap-1 px-4">
        <CardTitle className="text-sm">{title}</CardTitle>
        {description && <CardDescription className="text-xs leading-relaxed">{description}</CardDescription>}
        {aside && <CardAction>{aside}</CardAction>}
      </CardHeader>
      <CardContent className="px-4">{children}</CardContent>
    </Card>
  );
}

export function SettingsField({ label, hint, error, helpId, className = "", children }) {
  return (
    <label className={cn("flex flex-col gap-2", className)}>
      <Label asChild><span className="text-xs font-medium text-foreground/85">{label}</span></Label>
      {children}
      {(error || hint) && (
        <small id={helpId} className={cn("text-xs leading-snug", error ? "text-destructive" : "text-muted-foreground")}>{error || hint}</small>
      )}
    </label>
  );
}

export function SettingsNumberField({ field, label, hint, value, error, onChange }) {
  const helpId = `${field}-help`;
  return (
    <SettingsField label={label} hint={hint} error={error} helpId={helpId}>
      <Input type="number" value={value} aria-invalid={Boolean(error)} aria-describedby={helpId} onChange={(event) => onChange(event.target.value)} />
    </SettingsField>
  );
}

const EMPTY_VALUE = "__oneshot_empty__";
const toRadix = (value) => (value === "" || value == null ? EMPTY_VALUE : String(value));
const fromRadix = (value) => (value === EMPTY_VALUE ? "" : value);

export function SettingsSelect({ value, onChange, options = [], ariaLabel, disabled = false, className = "" }) {
  const normalized = useMemo(() => options.map((option) => typeof option === "object" ? { ...option, value: toRadix(option.value) } : { value: toRadix(option), label: String(option) }), [options]);
  const current = toRadix(value);
  const selected = normalized.find((option) => option.value === current);
  return (
    <Select value={selected ? current : undefined} onValueChange={(next) => onChange(fromRadix(next))} disabled={disabled}>
      <SelectTrigger aria-label={ariaLabel} className={cn("w-full", className)}><SelectValue placeholder="—" /></SelectTrigger>
      <SelectContent>
        {normalized.map((option, index) => (
          <SelectItem key={`${option.value}-${index}`} value={option.value} disabled={option.disabled}>
            <span className="truncate" title={option.label}>{option.label}</span>
            {option.meta && <span className="ml-auto max-w-[42%] truncate text-xs text-muted-foreground" title={option.meta}>{option.meta}</span>}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

export function SettingsSwitchRow({ checked, onChange, label, description, dangerous = false, disabled = false }) {
  const id = useId();
  return (
    <div className={cn("mt-2 flex items-center gap-4 rounded-xl bg-muted/45 px-4 py-3.5 first:mt-0", disabled && "opacity-50")}>
      <span className="min-w-0 flex-1">
        <Label htmlFor={id} className={cn("block text-sm font-semibold", dangerous ? "text-destructive" : "text-foreground")}>{label}</Label>
        {description && <small className="mt-1 block text-xs leading-relaxed text-muted-foreground">{description}</small>}
      </span>
      <Switch id={id} checked={Boolean(checked)} disabled={disabled} aria-label={typeof label === "string" ? label : undefined} onCheckedChange={onChange} className={cn(dangerous && "data-[state=checked]:bg-destructive")} />
    </div>
  );
}
