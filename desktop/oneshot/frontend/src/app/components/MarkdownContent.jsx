import { memo } from "react";
import { useTranslation } from "react-i18next";
import ReactMarkdown from "react-markdown";
import rehypeSanitize from "rehype-sanitize";
import remarkGfm from "remark-gfm";

function SafeLink({ href = "", children, node: _node, ...props }) {
  const external = /^(?:https?:|mailto:)/i.test(href);
  if (!external && !href.startsWith("#")) return <span className="markdown-unsafe-link" title={href}>{children}</span>;
  return <a {...props} href={href} target={external ? "_blank" : undefined} rel={external ? "noreferrer noopener" : undefined}>{children}</a>;
}

// Agent output is untrusted. ReactMarkdown does not enable raw HTML, the
// sanitizer constrains the generated tree, and images are represented without
// fetching their URLs from the desktop webview.
function MarkdownContent({ content, streaming = false, className = "" }) {
  const { t } = useTranslation();
  return <div className={`markdown-content ${streaming ? "streaming" : ""} ${className}`.trim()} aria-busy={streaming || undefined}>
    <ReactMarkdown
      skipHtml
      remarkPlugins={[remarkGfm]}
      rehypePlugins={[rehypeSanitize]}
      components={{
        a: SafeLink,
        img: ({ alt = "" }) => <span className="markdown-image-placeholder">{t("markdown.image", { alt: alt ? `: ${alt}` : "" })}</span>,
      }}
    >{String(content || "")}</ReactMarkdown>
    {streaming && <span className="markdown-stream-cursor" aria-hidden="true" />}
  </div>;
}

export default memo(MarkdownContent);
