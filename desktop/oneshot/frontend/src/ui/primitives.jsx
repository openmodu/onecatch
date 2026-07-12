export function Action({ children, tone = "accent", bracket = true, className = "", type = "button", ...props }) {
  return <button type={type} className={`ui-action ui-action--${tone} ${className}`.trim()} {...props}>{bracket ? <>[ {children} ]</> : children}</button>;
}

export function Kicker({ children, className = "" }) {
  return <span className={`ui-kicker ${className}`.trim()}>{children}</span>;
}

export function StatusBadge({ status = "accent", children, className = "" }) {
  return <span className={`ui-badge ui-badge--${status} ${className}`.trim()}>{children}</span>;
}

export function ModeBadge({ mode = "serial", children }) {
  return <StatusBadge status={mode}>{children || mode}</StatusBadge>;
}

export function Panel({ title, description, aside, children, className = "" }) {
  return <section className={`ui-panel ${className}`.trim()}>
    {(title || aside) && <div className="ui-panel__head"><h3 className="ui-panel__title">{title}</h3>{aside}</div>}
    {description && <p className="ui-panel__description">{description}</p>}
    {children}
  </section>;
}

export function SettingPanel(props) {
  return <Panel {...props} className={`setting-card ${props.className || ""}`.trim()} />;
}

export function Field({ label, hint, error, helpId, className = "", children }) {
  return <label className={`ui-field ${className} ${error ? "ui-field--error" : ""}`.trim()}>
    <span className="ui-field__label">{label}</span>
    {children}
    {(error || hint) && <small className="ui-field__hint" id={helpId}>{error || hint}</small>}
  </label>;
}

export function NumberField({ field, label, hint, value, error, onChange }) {
  const helpId = `${field}-help`;
  return <Field label={label} hint={hint} error={error} helpId={helpId}>
    <input type="number" value={value} aria-invalid={Boolean(error)} aria-describedby={helpId} onChange={(event) => onChange(event.target.value)} />
  </Field>;
}

export function TUISelect({ value, onChange, options = [], ariaLabel, disabled = false, className = "" }) {
  const id = useId().replace(/:/g, "");
  const triggerRef = useRef(null);
  const menuRef = useRef(null);
  const normalized = useMemo(() => options.map((option) => typeof option === "object" ? { ...option, value: String(option.value) } : { value: String(option), label: String(option) }), [options]);
  const selectedIndex = normalized.findIndex((option) => option.value === String(value ?? ""));
  const selected = normalized[selectedIndex] || normalized.find((option) => !option.disabled) || { value: "", label: "—" };
  const [open, setOpen] = useState(false);
  const [activeIndex, setActiveIndex] = useState(Math.max(selectedIndex, 0));
  const [menuStyle, setMenuStyle] = useState({});

  const enabledIndex = (from, direction) => {
    if (!normalized.length) return -1;
    let index = from;
    for (let count = 0; count < normalized.length; count += 1) {
      index = (index + direction + normalized.length) % normalized.length;
      if (!normalized[index].disabled) return index;
    }
    return -1;
  };

  const choose = (index) => {
    const option = normalized[index];
    if (!option || option.disabled) return;
    onChange(option.value);
    setOpen(false);
    triggerRef.current?.focus();
  };

  const openMenu = (direction = 1) => {
    const fallback = enabledIndex(direction > 0 ? -1 : 0, direction);
    setActiveIndex(selectedIndex >= 0 && !normalized[selectedIndex]?.disabled ? selectedIndex : fallback);
    setOpen(true);
  };

  useEffect(() => {
    if (!open) return undefined;
    const updatePosition = () => {
      const rect = triggerRef.current?.getBoundingClientRect();
      if (!rect) return;
      const estimatedHeight = Math.min(248, normalized.length * 36 + 8);
      const openAbove = window.innerHeight - rect.bottom < estimatedHeight && rect.top > estimatedHeight;
      setMenuStyle({ left: rect.left, top: openAbove ? rect.top - estimatedHeight - 4 : rect.bottom + 4, width: rect.width });
    };
    const closeOutside = (event) => {
      if (!triggerRef.current?.contains(event.target) && !menuRef.current?.contains(event.target)) setOpen(false);
    };
    updatePosition();
    document.addEventListener("pointerdown", closeOutside, true);
    window.addEventListener("resize", updatePosition);
    window.addEventListener("scroll", updatePosition, true);
    return () => {
      document.removeEventListener("pointerdown", closeOutside, true);
      window.removeEventListener("resize", updatePosition);
      window.removeEventListener("scroll", updatePosition, true);
    };
  }, [normalized.length, open]);

  const handleKeyDown = (event) => {
    if (["ArrowDown", "ArrowUp", "Home", "End", "Enter", " ", "Escape"].includes(event.key)) event.preventDefault();
    if (event.key === "Escape") { setOpen(false); return; }
    if (event.key === "Enter" || event.key === " ") {
      if (open) choose(activeIndex); else openMenu();
      return;
    }
    if (event.key === "Home" || event.key === "End") {
      if (!open) setOpen(true);
      setActiveIndex(enabledIndex(event.key === "Home" ? -1 : 0, event.key === "Home" ? 1 : -1));
      return;
    }
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      const direction = event.key === "ArrowDown" ? 1 : -1;
      if (!open) openMenu(direction);
      else setActiveIndex((current) => enabledIndex(current, direction));
    }
  };

  return <div className={`ui-select ${open ? "ui-select--open" : ""} ${className}`.trim()}>
    <button ref={triggerRef} type="button" className="ui-select__trigger" role="combobox" aria-label={ariaLabel} aria-controls={`${id}-listbox`} aria-expanded={open} aria-haspopup="listbox" aria-activedescendant={open && activeIndex >= 0 ? `${id}-option-${activeIndex}` : undefined} disabled={disabled} onClick={() => open ? setOpen(false) : openMenu()} onKeyDown={handleKeyDown}>
      <span className="ui-select__value">{selected.label}</span><span className="ui-select__caret" aria-hidden="true">{open ? "▴" : "▾"}</span>
    </button>
    {open && createPortal(<div ref={menuRef} id={`${id}-listbox`} className="ui-select__menu" role="listbox" aria-label={ariaLabel} style={menuStyle}>
      {normalized.map((option, index) => <div id={`${id}-option-${index}`} key={`${option.value}-${index}`} className={`ui-select__option ${index === activeIndex ? "active" : ""} ${option.value === String(value ?? "") ? "selected" : ""} ${option.disabled ? "disabled" : ""}`.trim()} role="option" aria-selected={option.value === String(value ?? "")} aria-disabled={Boolean(option.disabled)} onPointerMove={() => !option.disabled && setActiveIndex(index)} onPointerDown={(event) => event.preventDefault()} onClick={() => choose(index)}><span className="ui-select__mark" aria-hidden="true">{option.value === String(value ?? "") ? ">" : ""}</span><span>{option.label}</span>{option.meta && <small>{option.meta}</small>}</div>)}
    </div>, document.body)}
  </div>;
}

export function ToggleRow({ checked, onChange, label, description, dangerous = false, disabled = false }) {
  return <button type="button" role="switch" aria-checked={Boolean(checked)} disabled={disabled} className={`ui-toggle-row ${checked ? "ui-toggle-row--checked" : ""} ${dangerous ? "ui-toggle-row--danger" : ""}`.trim()} onClick={() => onChange(!checked)}>
    <span><strong>{label}</strong><small>{description}</small></span>
    <span className="ui-toggle-row__state">[ {checked ? "on" : "off"} ]</span>
  </button>;
}

export function Toolbar({ children, className = "" }) {
  return <header className={`ui-toolbar ${className}`.trim()}>{children}</header>;
}

export function ToolbarSpacer() {
  return <span className="ui-toolbar__spacer" aria-hidden="true" />;
}
import { useEffect, useId, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
