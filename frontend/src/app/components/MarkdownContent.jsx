import { cloneElement, isValidElement, memo } from "react";
import { Browser } from "@wailsio/runtime";
import { useTranslation } from "react-i18next";
import { Streamdown } from "streamdown";

function SafeLink({ href = "", children, node: _node, ...props }) {
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

const MARKDOWN_COMPONENTS = {
  a: SafeLink,
  code: PlainCode,
  img: ImagePlaceholder,
  pre: PlainPre,
};

const LINK_SAFETY = { enabled: false };

// Agent output is untrusted. Streamdown sanitizes and hardens its generated
// tree by default; raw HTML stays disabled here, while images remain inert
// placeholders so the desktop webview never fetches model-provided URLs.
function MarkdownContent({ content, streaming = false, className = "" }) {
  const text = String(content || "");
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
    skipHtml
  >{text}</Streamdown>;
}

export default memo(MarkdownContent);
