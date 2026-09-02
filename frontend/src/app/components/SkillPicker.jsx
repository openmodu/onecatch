import { useCallback, useEffect, useId, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { RuntimeBinding } from "../../../bindings/github.com/openmodu/onecatch/internal/transport/wails/index.js";
import { filterSkills, findSkillTrigger, insertSkill } from "../skillMention.js";

const skillCache = new Map();
const demoSkills = [
  { name: "git-commit", displayName: "Git Commit", shortDescription: "Create a clean, repo-aware commit" },
  { name: "openai-docs", displayName: "OpenAI Docs", shortDescription: "Look up official Codex and OpenAI documentation" },
  { name: "skill-creator", displayName: "Skill Creator", shortDescription: "Create or update a Codex Skill" },
];

function cachedSkills(mode, runtime, workspacePath) {
  if (mode !== "wails") return Promise.resolve(demoSkills);
  const key = `${runtime}:${workspacePath || "__home__"}`;
  if (!skillCache.has(key)) {
    const request = RuntimeBinding.ListSkills(runtime, workspacePath || "")
      .then((items) => Array.isArray(items) ? items : [])
      .catch((error) => {
        skillCache.delete(key);
        throw error;
      });
    skillCache.set(key, request);
  }
  return skillCache.get(key);
}

export function useSkillPicker({ enabled, mode, runtime, workspacePath, value, onValueChange, textareaRef, onKeyDown }) {
  const { t } = useTranslation();
  const menuID = `skill-picker-${useId().replaceAll(":", "")}`;
  const [trigger, setTrigger] = useState(null);
  const [skills, setSkills] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [activeIndex, setActiveIndex] = useState(0);
  const loadVersion = useRef(0);

  useEffect(() => {
    loadVersion.current += 1;
    setTrigger(null);
    setSkills(null);
    setLoading(false);
    setError("");
  }, [enabled, mode, runtime, workspacePath]);

  const ensureSkills = useCallback(() => {
    if (!enabled || skills || loading) return;
    const version = loadVersion.current;
    setLoading(true);
    setError("");
    void cachedSkills(mode, runtime, workspacePath).then((items) => {
      if (version !== loadVersion.current) return;
      setSkills(items);
      setLoading(false);
    }).catch(() => {
      if (version !== loadVersion.current) return;
      setError(t("skills.loadFailed"));
      setLoading(false);
    });
  }, [enabled, loading, mode, runtime, skills, t, workspacePath]);

  const updateTrigger = useCallback((nextValue, caret) => {
    if (!enabled) {
      setTrigger(null);
      return;
    }
    const nextTrigger = findSkillTrigger(nextValue, caret);
    setTrigger(nextTrigger);
    if (nextTrigger) ensureSkills();
  }, [enabled, ensureSkills]);

  const filtered = useMemo(() => filterSkills(skills || [], trigger?.query || ""), [skills, trigger?.query]);

  useEffect(() => {
    setActiveIndex(0);
  }, [trigger?.query, skills]);

  const selectSkill = useCallback((skill) => {
    if (!trigger) return;
    const inserted = insertSkill(value, trigger, skill.name);
    onValueChange(inserted.value);
    setTrigger(null);
    window.requestAnimationFrame(() => {
      textareaRef.current?.focus();
      textareaRef.current?.setSelectionRange(inserted.caret, inserted.caret);
    });
  }, [onValueChange, textareaRef, trigger, value]);

  const handleChange = useCallback((event) => {
    const nextValue = event.target.value;
    const caret = event.target.selectionStart ?? nextValue.length;
    onValueChange(nextValue);
    updateTrigger(nextValue, caret);
  }, [onValueChange, updateTrigger]);

  const handleKeyDown = useCallback((event) => {
    if (trigger && !event.nativeEvent?.isComposing) {
      if (event.key === "Escape") {
        event.preventDefault();
        setTrigger(null);
        return;
      }
      if (event.key === "ArrowDown" || event.key === "ArrowUp") {
        event.preventDefault();
        if (filtered.length) {
          const delta = event.key === "ArrowDown" ? 1 : -1;
          setActiveIndex((current) => (current + delta + filtered.length) % filtered.length);
        }
        return;
      }
      if (event.key === "Enter" || event.key === "Tab") {
        event.preventDefault();
        if (filtered[activeIndex]) selectSkill(filtered[activeIndex]);
        return;
      }
    }
    onKeyDown?.(event);
  }, [activeIndex, filtered, onKeyDown, selectSkill, trigger]);

  const handleBlur = useCallback(() => {
    window.setTimeout(() => setTrigger(null), 0);
  }, []);

  const menu = enabled && trigger ? <div id={menuID} className="codex-skill-menu" role="listbox" aria-label={t("skills.listLabel")}>
    <div className="codex-skill-menu-header"><span>{t("skills.title")}</span><kbd>$</kbd></div>
    <div className="codex-skill-menu-list">
      {loading && <div className="codex-skill-menu-state">{t("skills.loading")}</div>}
      {!loading && error && <div className="codex-skill-menu-state error">{error}</div>}
      {!loading && !error && filtered.length === 0 && <div className="codex-skill-menu-state">{t("skills.empty")}</div>}
      {!loading && !error && filtered.map((skill, index) => <button
        type="button"
        className={`codex-skill-option ${index === activeIndex ? "active" : ""}`.trim()}
        role="option"
        aria-selected={index === activeIndex}
        key={`${skill.name}-${skill.path || index}`}
        onMouseEnter={() => setActiveIndex(index)}
        onMouseDown={(event) => event.preventDefault()}
        onClick={() => selectSkill(skill)}
      >
        <span className="codex-skill-option-name">${skill.name}</span>
        <span className="codex-skill-option-description">{skill.shortDescription || skill.description || skill.displayName || ""}</span>
      </button>)}
    </div>
  </div> : null;

  return {
    menu,
    inputProps: {
      onChange: handleChange,
      onKeyDown: handleKeyDown,
      onBlur: handleBlur,
      "aria-autocomplete": enabled ? "list" : undefined,
      "aria-controls": trigger ? menuID : undefined,
      "aria-expanded": enabled ? Boolean(trigger) : undefined,
    },
  };
}
