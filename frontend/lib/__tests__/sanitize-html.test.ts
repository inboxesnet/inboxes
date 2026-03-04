import { describe, it, expect, beforeEach } from "vitest";
import DOMPurify from "dompurify";
import { sanitizeEmailHtml } from "../sanitize-html";

// DOMPurify hooks persist between calls — clear them before each test
beforeEach(() => {
  DOMPurify.removeAllHooks();
});

describe("sanitizeEmailHtml", () => {
  // ── XSS Protection ──

  it("strips <script> tags", () => {
    const { html } = sanitizeEmailHtml(
      '<p>Hello</p><script>alert("xss")</script>',
      false
    );
    expect(html).not.toContain("<script");
    expect(html).toContain("<p>Hello</p>");
  });

  it("strips <form> tags", () => {
    const { html } = sanitizeEmailHtml(
      '<form action="/steal"><input type="text" /></form><p>Safe</p>',
      false
    );
    expect(html).not.toContain("<form");
    expect(html).not.toContain("<input");
    expect(html).toContain("Safe");
  });

  it("strips <iframe> tags", () => {
    const { html } = sanitizeEmailHtml(
      '<iframe src="https://evil.com"></iframe><p>Content</p>',
      false
    );
    expect(html).not.toContain("<iframe");
    expect(html).toContain("Content");
  });

  it("strips event handlers (onclick, onerror, onload)", () => {
    const { html } = sanitizeEmailHtml(
      '<p onclick="alert(1)">Click</p><img onerror="alert(2)" src="x" /><div onload="alert(3)">Hi</div>',
      false
    );
    expect(html).not.toContain("onclick");
    expect(html).not.toContain("onerror");
    expect(html).not.toContain("onload");
  });

  // ── Safe Tags ──

  it("allows safe tags (p, strong, em, a, img, table, ul, ol, li, blockquote)", () => {
    const input =
      '<p><strong>Bold</strong> <em>Italic</em></p>' +
      '<a href="https://example.com">Link</a>' +
      '<ul><li>Item</li></ul>' +
      '<ol><li>Item</li></ol>' +
      '<blockquote>Quote</blockquote>' +
      '<table><tr><td>Cell</td></tr></table>';
    const { html } = sanitizeEmailHtml(input, true);
    expect(html).toContain("<p>");
    expect(html).toContain("<strong>");
    expect(html).toContain("<em>");
    expect(html).toContain("<a ");
    expect(html).toContain("<ul>");
    expect(html).toContain("<ol>");
    expect(html).toContain("<li>");
    expect(html).toContain("<blockquote>");
    expect(html).toContain("<table>");
    expect(html).toContain("<td>");
  });

  // ── CSS Sanitization ──

  it("allows safe CSS properties (color, font-size, margin, padding)", () => {
    const { html } = sanitizeEmailHtml(
      '<p style="color: red; font-size: 14px; margin: 10px; padding: 5px;">Styled</p>',
      false
    );
    expect(html).toContain("color");
    expect(html).toContain("font-size");
  });

  it("blocks CSS url() in style attributes", () => {
    const { html } = sanitizeEmailHtml(
      '<div style="background-color: url(https://tracker.com/pixel.gif);">Content</div>',
      false
    );
    expect(html).not.toContain("url(");
  });

  // ── Image Blocking ──

  it("blocks external images when showImages=false (src removed, data-original-src set)", () => {
    const { html, hasBlockedImages } = sanitizeEmailHtml(
      '<img src="https://tracker.com/pixel.gif" alt="Track" />',
      false
    );
    expect(hasBlockedImages).toBe(true);
    // The img should NOT have a direct src attribute pointing to the external URL
    // (data-original-src will contain it, but src should be removed)
    const imgEl = new DOMParser().parseFromString(html, "text/html").querySelector("img");
    expect(imgEl).toBeTruthy();
    expect(imgEl!.getAttribute("src")).toBeNull();
    expect(imgEl!.getAttribute("data-original-src")).toBe("https://tracker.com/pixel.gif");
    expect(html).toContain("[Image blocked for privacy]");
  });

  it("allows data: URI images (not blocked)", () => {
    const dataUri = "data:image/png;base64,iVBORw0KGgo=";
    const { html, hasBlockedImages } = sanitizeEmailHtml(
      `<img src="${dataUri}" alt="Inline" />`,
      false
    );
    // data: URIs are not external — should not be blocked
    expect(hasBlockedImages).toBe(false);
    expect(html).toContain(dataUri);
  });

  it("allows cid: URI images (not blocked)", () => {
    const { html, hasBlockedImages } = sanitizeEmailHtml(
      '<img src="cid:image001@01D00000.00000000" alt="Embedded" />',
      false
    );
    expect(hasBlockedImages).toBe(false);
    expect(html).toContain("cid:");
  });

  it("restores images when showImages=true (external images kept)", () => {
    const { html, hasBlockedImages } = sanitizeEmailHtml(
      '<img src="https://cdn.example.com/photo.jpg" alt="Photo" />',
      true
    );
    expect(hasBlockedImages).toBe(true);
    expect(html).toContain('src="https://cdn.example.com/photo.jpg"');
    expect(html).not.toContain("data-original-src");
  });

  it("returns hasBlockedImages=true when external images exist", () => {
    const { hasBlockedImages } = sanitizeEmailHtml(
      '<p>Text</p><img src="https://example.com/img.png" />',
      false
    );
    expect(hasBlockedImages).toBe(true);
  });
});
