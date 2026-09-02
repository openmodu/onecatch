import { cloneElement, isValidElement, memo } from "react";
import { Browser } from "@wailsio/runtime";
import { useTranslation } from "react-i18next";
import { Streamdown } from "streamdown";
import { defaultRemarkPlugins } from "streamdown";
import { remarkSkillMentions } from "../skillMention.js";

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
// these overrides only avoid introducing a second code-block toolbar/style.
function PlainCode({ node: _node, children, ...props }) {
  return <code {...props}>{children}</code>;
}

function PlainPre({ node: _node, children, ...props }) {
  const content = isValidElement(children) ? cloneElement(children, { "data-block": "" }) : children;
  return <pre {...props}>{content}</pre>;
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
