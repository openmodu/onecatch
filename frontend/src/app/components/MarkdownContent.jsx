import { cloneElement, isValidElement, memo, useEffect, useMemo, useRef, useState } from "react";
import { Browser, Clipboard } from "@wailsio/runtime";
import { useTranslation } from "react-i18next";
import { Check, CodeXml, Copy, TriangleAlert, WrapText } from "lucide-react";
import { Streamdown } from "streamdown";
import { defaultRemarkPlugins } from "streamdown";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { remarkSkillMentions } from "../skillMention.js";
import { codeLanguageFromClassName, highlightCode } from "../syntaxHighlight.js";

function SafeLink({ href = "", children, node: _node, ...props }) {
  if (href.startsWith("#onecatch-skill:")) return <span className="codex-skill-mention">{children}</span>;
  const external = /^(?:https?:|mailto:)/i.test(href);
  if (!external && !href.startsWith("#")) return <span className="markdown-unsafe-link" title={href}>{children}</span>;
  const openExternal = external ? (event) => {
    event.preventDefault();
    void Browser.OpenURL(href).catch((error) => console.error("Failed to open external link", error));
  } : undefined;
  return <a {...props} href={href} onClick={openExternal} target={external ? "_blank" : undefined} rel={external ? "noreferrer noopener" : undefined}>{children}</a>;
}

function ImagePlaceholder({ alt = "" }) {
  const { t } = useTranslation();
  return <span className="markdown-image-placeholder">{t("markdown.image", { alt: alt ? `: ${alt}` : "" })}</span>;
}

// Keep fenced code in the application's existing, deliberately quiet visual
// treatment. Streamdown still repairs incomplete fences and memoizes blocks;
// these overrides keep the code-block toolbar/style application-owned.
function PlainCode({ node: _node, children, ...props }) {
  const block = props["data-block"] !== undefined;
  const language = codeLanguageFromClassName(props.className);
  const highlighted = useMemo(() => block && typeof children === "string" ? highlightCode(children, language) : null, [block, children, language]);
  // Prism escapes the source before adding token spans. Raw Markdown HTML
  // remains disabled; inline code keeps the ordinary React text renderer.
  if (highlighted) return <code {...props} data-language={highlighted.language} dangerouslySetInnerHTML={{ __html: highlighted.html }} />;
  return <code {...props}>{children}</code>;
}

function PlainPre({ node: _node, children, ...props }) {
  const { t } = useTranslation();
  const preRef = useRef(null);
  const [copyState, setCopyState] = useState("idle");
  const [wrapped, setWrapped] = useState(false);
  const language = isValidElement(children) ? codeLanguageFromClassName(children.props.className) : "";
  const languageLabel = language ? language.toUpperCase() : t("markdown.plainText");
  const wrapLabel = wrapped ? t("markdown.disableWrap") : t("markdown.enableWrap");
  useEffect(() => {
    if (copyState !== "copied" && copyState !== "error") return undefined;
    const timer = window.setTimeout(() => setCopyState("idle"), 2000);
    return () => window.clearTimeout(timer);
  }, [copyState]);

  const copyCode = async () => {
    // Read only the code at click time, including the latest streaming text.
    // Keep whitespace intact and leave toolbar labels out of the clipboard.
    const text = preRef.current?.textContent ?? "";
    setCopyState("copying");
    try {
      try {
        await Clipboard.SetText(text);
      } catch (wailsError) {
        if (!navigator.clipboard?.writeText) throw wailsError;
        await navigator.clipboard.writeText(text);
      }
      setCopyState("copied");
    } catch {
      setCopyState("error");
    }
  };
  const copyLabel = copyState === "copied" ? t("markdown.codeCopied")
    : copyState === "error" ? t("markdown.codeCopyFailed")
      : copyState === "copying" ? t("markdown.copyingCode") : t("markdown.copyCode");
  const CopyIcon = copyState === "copied" ? Check : copyState === "error" ? TriangleAlert : Copy;
  const content = isValidElement(children) ? cloneElement(children, { "data-block": "" }) : children;
  return <div className={`markdown-code-block${wrapped ? " is-wrapped" : ""}`}>
    <div className="markdown-code-toolbar">
      <span className="markdown-code-language"><CodeXml aria-hidden="true" /><span title={languageLabel}>{languageLabel}</span></span>
      <div className="markdown-code-actions">
        <Tooltip><TooltipTrigger asChild><Button type="button" variant="ghost" size="icon-sm" className="markdown-code-wrap" aria-label={wrapLabel} aria-pressed={wrapped} onClick={() => setWrapped((current) => !current)}><WrapText aria-hidden="true" /></Button></TooltipTrigger><TooltipContent side="top" sideOffset={6}>{wrapLabel}</TooltipContent></Tooltip>
        <Tooltip><TooltipTrigger asChild><Button type="button" variant="ghost" size="icon-sm" className="markdown-code-copy" aria-label={copyLabel} disabled={copyState === "copying"} onClick={copyCode}><CopyIcon aria-hidden="true" /></Button></TooltipTrigger><TooltipContent side="top" sideOffset={6}>{copyLabel}</TooltipContent></Tooltip>
        <span className="sr-only" role="status" aria-live="polite">{copyState === "copied" || copyState === "error" ? copyLabel : ""}</span>
      </div>
    </div>
    <pre {...props} ref={preRef}>{content}</pre>
  </div>;
}

function PlainH1({ node: _node, ...props }) { return <h1 {...props} />; }
function PlainH2({ node: _node, ...props }) { return <h2 {...props} />; }
function PlainH3({ node: _node, ...props }) { return <h3 {...props} />; }
function PlainH4({ node: _node, ...props }) { return <h4 {...props} />; }

// Streamdown's default table renderer adds a padded card around its scrolling
// frame even when controls are disabled. The transcript already owns the
// document surface, so keep only one quiet border and one responsive scroller.
function PlainTable({ node: _node, children, ...props }) {
  return <div className="markdown-table-scroll"><table {...props}>{children}</table></div>;
}

const MARKDOWN_COMPONENTS = {
  a: SafeLink,
  code: PlainCode,
  h1: PlainH1,
  h2: PlainH2,
  h3: PlainH3,
  h4: PlainH4,
  img: ImagePlaceholder,
  pre: PlainPre,
  table: PlainTable,
};

const LINK_SAFETY = { enabled: false };
const REMARK_PLUGINS = [...Object.values(defaultRemarkPlugins), remarkSkillMentions];

// Agent output is untrusted. Streamdown sanitizes and hardens its generated
// tree by default; raw HTML stays disabled here, while images remain inert
// placeholders so the desktop webview never fetches model-provided URLs.
function MarkdownContent({ content, streaming = false, className = "" }) {
  const text = String(content || "");
  // A trailing newline is meaningful to the accumulated transcript, but while
  // streaming `white-space: pre-wrap` puts Streamdown's caret on a blank line.
  // Trim only the display copy; the next frame still arrives with the original
  // bytes and static output is never altered.
  const renderedText = streaming ? text.replace(/\s+$/u, "") : text;
  return <Streamdown
    aria-busy={streaming || undefined}
    caret={streaming ? "block" : undefined}
    className={`markdown-content ${streaming ? "streaming" : ""} ${className}`.trim()}
    components={MARKDOWN_COMPONENTS}
    controls={false}
    isAnimating={streaming}
    lineNumbers={false}
    linkSafety={LINK_SAFETY}
    mode={streaming ? "streaming" : "static"}
    remarkPlugins={REMARK_PLUGINS}
    skipHtml
  >{renderedText}</Streamdown>;
}

export default memo(MarkdownContent);
