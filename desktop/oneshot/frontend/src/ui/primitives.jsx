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
