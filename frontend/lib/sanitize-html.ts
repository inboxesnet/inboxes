import DOMPurify from "dompurify";
import { sanitizeLinkNode } from "./sanitize-links";

export const ALLOWED_CSS_PROPERTIES = new Set([
  'color', 'background-color', 'font-size', 'font-weight',
  'font-family', 'font-style', 'text-align', 'text-decoration',
  'line-height', 'letter-spacing', 'word-spacing',
  'margin', 'margin-top', 'margin-right', 'margin-bottom', 'margin-left',
  'padding', 'padding-top', 'padding-right', 'padding-bottom', 'padding-left',
  'border', 'border-radius', 'border-color', 'border-style', 'border-width',
  'border-top', 'border-right', 'border-bottom', 'border-left',
  'width', 'height', 'max-width', 'max-height', 'min-width', 'min-height',
  'display', 'vertical-align', 'list-style-type', 'white-space',
  'overflow', 'text-overflow', 'word-break',
  'table-layout', 'border-collapse', 'border-spacing',
]);

export const ALLOWED_TAGS = [
  "p", "br", "strong", "em", "u", "a", "ul", "ol", "li",
  "h1", "h2", "h3", "h4", "blockquote", "pre", "code",
  "img", "table", "thead", "tbody", "tr", "td", "th",
  "div", "span",
];

export const ALLOWED_ATTR = [
  "href", "src", "alt", "style", "class", "target", "rel", "width", "height",
  "data-original-src", "dir",
];

export function sanitizeEmailHtml(html: string, showImages: boolean, stripTracking = true) {
  let hasBlockedImages = false;

  DOMPurify.addHook('afterSanitizeAttributes', (node) => {
    // Block external images (tracking pixels)
    if (node.tagName === 'IMG') {
      const src = node.getAttribute('src');
      if (src && !src.startsWith('data:') && !src.startsWith('cid:')) {
        hasBlockedImages = true;
        if (!showImages) {
          node.setAttribute('data-original-src', src);
          node.removeAttribute('src');
          node.setAttribute('alt', '[Image blocked for privacy]');
        }
      }
    }

    // Open links in new tab + optionally strip tracking params
    sanitizeLinkNode(node, stripTracking);

    // Sanitize CSS against allowlist
    if (node.hasAttribute('style')) {
      const style = (node as HTMLElement).style;
      const safeStyles: string[] = [];
      for (let i = 0; i < style.length; i++) {
        const prop = style[i];
        if (ALLOWED_CSS_PROPERTIES.has(prop)) {
          const value = style.getPropertyValue(prop);
          if (!value.includes('url(')) {
            safeStyles.push(`${prop}: ${value}`);
          }
        }
      }
      if (safeStyles.length > 0) {
        node.setAttribute('style', safeStyles.join('; '));
      } else {
        node.removeAttribute('style');
      }
    }
  });

  const result = DOMPurify.sanitize(html, {
    ALLOWED_TAGS,
    ALLOWED_ATTR,
  });

  DOMPurify.removeAllHooks();
  return { html: result, hasBlockedImages };
}
