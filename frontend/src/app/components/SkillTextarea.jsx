import { forwardRef, useCallback, useLayoutEffect, useRef } from "react";
import { splitSkillMentions } from "../skillMention.js";

export function SkillMentionText({ value = "" }) {
  return splitSkillMentions(value).map((part, index) => part.type === "skill"
    ? <span className="codex-skill-mention" data-skill-name={part.name} key={`${part.name}-${index}`}>{part.value}</span>
    : <span key={`text-${index}`}>{part.value}</span>);
}

const SkillTextarea = forwardRef(function SkillTextarea({
  highlight = true,
  textareaComponent: TextareaComponent = "textarea",
  value = "",
  onScroll,
  className = "",
  ...props
}, forwardedRef) {
  const textareaRef = useRef(null);
  const mirrorRef = useRef(null);
  const setTextareaRef = useCallback((node) => {
    textareaRef.current = node;
    if (typeof forwardedRef === "function") forwardedRef(node);
    else if (forwardedRef) forwardedRef.current = node;
  }, [forwardedRef]);
  const syncScroll = (event) => {
    if (mirrorRef.current) {
      mirrorRef.current.scrollTop = event.currentTarget.scrollTop;
      mirrorRef.current.scrollLeft = event.currentTarget.scrollLeft;
    }
    onScroll?.(event);
  };

  useLayoutEffect(() => {
    if (!textareaRef.current || !mirrorRef.current) return;
    mirrorRef.current.scrollTop = textareaRef.current.scrollTop;
    mirrorRef.current.scrollLeft = textareaRef.current.scrollLeft;
  }, [value]);

  return <>
    {highlight && <div ref={mirrorRef} className={`codex-skill-input-mirror ${className}`.trim()} aria-hidden="true">
      <SkillMentionText value={value} />
      {String(value).endsWith("\n") && <span>{"\u200b"}</span>}
    </div>}
    <TextareaComponent
      {...props}
      ref={setTextareaRef}
      className={`${className} ${highlight ? "codex-skill-textarea" : ""}`.trim()}
      value={value}
      onScroll={syncScroll}
    />
  </>;
});

export default SkillTextarea;
