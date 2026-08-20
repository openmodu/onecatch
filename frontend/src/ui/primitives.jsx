import { useId, useMemo } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { cn } from "@/lib/utils";

/* This module keeps the call signatures the rest of the app already uses
   (tone="danger", size="compact", options=[{value,label,meta}], …) and renders
   shadcn underneath, so the migration doesn't have to touch 16 call sites at
   once. The shadcn files in components/ui stay pristine — every deviation is
   expressed as a className here so `shadcn add --overwrite` stays safe. */

const ACTION_VARIANT = {
  primary: "default",
  accent: "secondary",
  muted: "outline",
  danger: "outline",
  cyan: "outline",
};

const ACTION_TONE = {
  danger: "border-destructive/40 text-destructive hover:bg-destructive hover:text-destructive-foreground",
  cyan: "border-info/40 text-info hover:bg-info hover:text-info-foreground",
};

const ACTION_SIZE = { regular: "sm", compact: "xs" };

export function Action({ children, tone = "accent", size = "regular", className = "", type = "button", ...props }) {
  return (
    <Button
      type={type}
      variant={ACTION_VARIANT[tone] || "secondary"}
      size={ACTION_SIZE[size] || "sm"}
      className={cn(ACTION_TONE[tone], className)}
      {...props}
    >
      {children}
    </Button>
  );
}

export function Kicker({ children, className = "" }) {
  return (
    <span className={cn("text-[11px] font-semibold uppercase tracking-[0.08em] text-muted-foreground", className)}>
      {children}
    </span>
  );
}

/* Run states carry more meaning than shadcn's default/secondary/destructive
   trio, so each maps onto one of the semantic colour tokens. Anything not
   listed (ready, pending, skipped, an unknown backend state) falls through to
   neutral rather than borrowing the identity colour. */
const TONE_PRIMARY = "border-primary/35 bg-primary/10 text-primary";
const TONE_SUCCESS = "border-success/35 bg-success/10 text-success";
const TONE_WARNING = "border-warning/35 bg-warning/10 text-warning";
const TONE_DANGER = "border-destructive/35 bg-destructive/10 text-destructive";
const TONE_INFO = "border-info/35 bg-info/10 text-info";
const TONE_NEUTRAL = "border-border bg-muted text-muted-foreground";

const BADGE_TONE = {
  accent: TONE_PRIMARY,
  running: TONE_PRIMARY,
  serial: TONE_PRIMARY,
  good: TONE_SUCCESS,
  completed: TONE_SUCCESS,
  succeeded: TONE_SUCCESS,
  warn: TONE_WARNING,
  paused: TONE_WARNING,
  danger: TONE_DANGER,
  failed: TONE_DANGER,
  cancelled: TONE_DANGER,
  interrupted: TONE_DANGER,
  cyan: TONE_INFO,
  dag: TONE_INFO,
  queued: TONE_INFO,
};

export function StatusBadge({ status = "accent", children, className = "" }) {
  return (
    <Badge
      variant="outline"
      className={cn("gap-1.5 font-medium", BADGE_TONE[status] || TONE_NEUTRAL, className)}
    >
      {children}
    </Badge>
  );
}

export function ModeBadge({ mode = "serial", children }) {
  return <StatusBadge status={mode}>{children || mode}</StatusBadge>;
}

export function Panel({ title, description, aside, children, className = "", headingLevel = 3 }) {
  const Heading = `h${Math.min(6, Math.max(1, headingLevel))}`;
  return (
    <section className={cn("rounded-md bg-muted/35 p-4", className)}>
      {(title || aside) && (
        <div className="mb-1 flex items-center justify-between gap-3">
          <Heading className="m-0 text-sm font-semibold text-foreground">{title}</Heading>
          {aside}
        </div>
      )}
      {description && <p className="mt-0 mb-3 max-w-3xl text-xs leading-relaxed text-muted-foreground">{description}</p>}
      {children}
    </section>
  );
}

export function SettingPanel(props) {
  return <Panel {...props} />;
}

/* A settings section: heading and blurb sit outside the card, the controls
   sit inside it, so consecutive modules read as separate groups. */
export function SettingsModule({ title, description, aside, children, className = "", bodyClassName = "" }) {
  return (
    <section className={cn("settings-module mb-7", className)}>
      <div className="flex items-start justify-between gap-6 px-0.5 pb-3">
        <div className="min-w-0">
          <h3 className="m-0 text-base font-semibold leading-tight text-foreground">{title}</h3>
          {description && <p className="mt-1.5 mb-0 max-w-2xl text-xs leading-relaxed text-muted-foreground">{description}</p>}
        </div>
        {aside}
      </div>
      <div className={cn("settings-module-body overflow-hidden rounded-lg border bg-card", bodyClassName)}>{children}</div>
    </section>
  );
}

export function Field({ label, hint, error, helpId, className = "", children }) {
  return (
    <label className={cn("flex flex-col gap-2", className)}>
      <Label asChild>
        <span className="text-muted-foreground">{label}</span>
      </Label>
      {children}
      {(error || hint) && (
        <small id={helpId} className={cn("text-xs leading-snug", error ? "text-destructive" : "text-muted-foreground")}>
          {error || hint}
        </small>
      )}
    </label>
  );
}

export function NumberField({ field, label, hint, value, error, onChange }) {
  const helpId = `${field}-help`;
  return (
    <Field label={label} hint={hint} error={error} helpId={helpId}>
      <Input
        type="number"
        value={value}
        aria-invalid={Boolean(error)}
        aria-describedby={helpId}
        onChange={(event) => onChange(event.target.value)}
      />
    </Field>
  );
}

/* Radix throws on an empty-string item value, but several call sites use "" to
   mean "inherit the global default". Swap in a sentinel across the Radix
   boundary and translate it back on the way out. */
const EMPTY_VALUE = "__onecatch_empty__";
const toRadix = (value) => (value === "" || value == null ? EMPTY_VALUE : String(value));
const fromRadix = (value) => (value === EMPTY_VALUE ? "" : value);

export function TUISelect({ value, onChange, options = [], ariaLabel, disabled = false, className = "" }) {
  const normalized = useMemo(
    () =>
      options.map((option) =>
        typeof option === "object"
          ? { ...option, value: toRadix(option.value) }
          : { value: toRadix(option), label: String(option) },
      ),
    [options],
  );
  const current = toRadix(value);
  const selected = normalized.find((option) => option.value === current);

  return (
    <Select value={selected ? current : undefined} onValueChange={(next) => onChange(fromRadix(next))} disabled={disabled}>
      <SelectTrigger aria-label={ariaLabel} className={cn("w-full", className)}>
        <SelectValue placeholder="—" />
      </SelectTrigger>
      <SelectContent>
        {normalized.map((option, index) => (
          <SelectItem key={`${option.value}-${index}`} value={option.value} disabled={option.disabled} className="[&>span:last-child]:w-full [&>span:last-child]:min-w-0">
            <span className="min-w-0 flex-1 truncate" title={option.label}>{option.label}</span>
            {option.meta && (
              <span className="max-w-[50%] shrink-0 truncate text-xs text-muted-foreground" title={option.meta}>
                {option.meta}
              </span>
            )}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

export function ToggleRow({ checked, onChange, label, description, dangerous = false, disabled = false }) {
  const id = useId();
  return (
    <div className={cn("mt-2 flex items-center gap-4 rounded-md bg-muted/45 px-3 py-3 first:mt-0", disabled && "opacity-50")}>
      <span className="min-w-0 flex-1">
        <Label
          htmlFor={id}
          className={cn("block text-sm font-semibold", dangerous ? "text-destructive" : "text-foreground")}
        >
          {label}
        </Label>
        {description && <small className="mt-1 block text-xs leading-relaxed text-muted-foreground">{description}</small>}
      </span>
      <Switch
        id={id}
        checked={Boolean(checked)}
        disabled={disabled}
        aria-label={typeof label === "string" ? label : undefined}
        onCheckedChange={(next) => onChange(next)}
        className={cn(dangerous && "data-[state=checked]:bg-destructive")}
      />
    </div>
  );
}

export function Toolbar({ children, className = "" }) {
  return (
    <header className={cn("flex h-12 shrink-0 items-center gap-3 px-5", className)}>
      {children}
    </header>
  );
}

export function ToolbarSpacer() {
  return <span className="flex-1" aria-hidden="true" />;
}
