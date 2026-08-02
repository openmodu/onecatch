import { memo } from "react";
import { Browser } from "@wailsio/runtime";
import { useTranslation } from "react-i18next";
import ReactMarkdown from "react-markdown";
import rehypeSanitize from "rehype-sanitize";
import remarkGfm from "remark-gfm";

function SafeLink({ href = "", children, node: _node, ...props }) {
  const external = /^(?:https?:|mailto:)/i.test(href);
  if (!external && !href.startsWith("#")) return <span className="markdown-unsafe-link" title={href}>{children}</span>;
  const openExternal = external ? (event) => {
    event.preventDefault();
    void Browser.OpenURL(href).catch((error) => console.error("Failed to open external link", error));
  } : undefined;
  return <a {...props} href={href} onClick={openExternal} target={external ? "_blank" : undefined} rel={external ? "noreferrer noopener" : undefined}>{children}</a>;
}

// Agent output is untrusted. ReactMarkdown does not enable raw HTML, the
// sanitizer constrains the generated tree, and images are represented without
// fetching their URLs from the desktop webview.
function MarkdownContent({ content, streaming = false, className = "" }) {
  const { t } = useTranslation();
  const text = String(content || "");
  // While a message streams, its full text is re-fed here on every ~80ms flush.
  // Running the whole remark/rehype pipeline that often (and re-parsing an
  // ever-longer string) is the single biggest source of streaming jank, so the
  // in-flight message renders as cheap pre-wrapped plain text. Markdown is
  // parsed exactly once, when the message settles.
  if (streaming) {
    return <div className={`markdown-content markdown-plain streaming ${className}`.trim()} aria-busy>
      {text}
      <span className="markdown-stream-cursor" aria-hidden="true" />
    </div>;
  }
  return <div className={`markdown-content ${className}`.trim()}>
    <ReactMarkdown
      skipHtml
      remarkPlugins={[remarkGfm]}
      rehypePlugins={[rehypeSanitize]}
      components={{
        a: SafeLink,
        img: ({ alt = "" }) => <span className="markdown-image-placeholder">{t("markdown.image", { alt: alt ? `: ${alt}` : "" })}</span>,
      }}
    >{text}</ReactMarkdown>
  </div>;
}

export default memo(MarkdownContent);
