import { describe, it, expect } from "vitest";
import { cleanLinkHref, sanitizeLinkNode } from "../sanitize-links";

describe("cleanLinkHref", () => {
  it("removes UTM params (utm_source, utm_medium, utm_campaign)", () => {
    const url =
      "https://example.com/page?utm_source=newsletter&utm_medium=email&utm_campaign=spring";
    const cleaned = cleanLinkHref(url);
    expect(cleaned).toBe("https://example.com/page");
    expect(cleaned).not.toContain("utm_source");
    expect(cleaned).not.toContain("utm_medium");
    expect(cleaned).not.toContain("utm_campaign");
  });

  it("removes additional UTM params (utm_term, utm_content, utm_id)", () => {
    const url =
      "https://example.com/?utm_term=test&utm_content=banner&utm_id=123";
    const cleaned = cleanLinkHref(url);
    expect(cleaned).toBe("https://example.com/");
  });

  it("removes Facebook params (fbclid, fbc, __cft__, __tn__)", () => {
    const url =
      "https://example.com/page?fbclid=abc123&fbc=def456&__cft__=xyz&__tn__=789";
    const cleaned = cleanLinkHref(url);
    expect(cleaned).toBe("https://example.com/page");
  });

  it("removes Google params (gclid, _ga, _gl)", () => {
    const url = "https://example.com/?gclid=abc&_ga=123&_gl=456";
    const cleaned = cleanLinkHref(url);
    expect(cleaned).toBe("https://example.com/");
  });

  it("removes Microsoft params (msclkid)", () => {
    const url = "https://example.com/?msclkid=click123";
    const cleaned = cleanLinkHref(url);
    expect(cleaned).toBe("https://example.com/");
  });

  it("removes Twitter params (twclid)", () => {
    const url = "https://example.com/?twclid=tw123";
    const cleaned = cleanLinkHref(url);
    expect(cleaned).toBe("https://example.com/");
  });

  it("removes HubSpot params (_hsenc, _hsmi)", () => {
    const url = "https://example.com/?_hsenc=abc&_hsmi=def";
    const cleaned = cleanLinkHref(url);
    expect(cleaned).toBe("https://example.com/");
  });

  it("removes Mailchimp params (mc_cid, mc_eid)", () => {
    const url = "https://example.com/?mc_cid=abc&mc_eid=def";
    const cleaned = cleanLinkHref(url);
    expect(cleaned).toBe("https://example.com/");
  });

  it("preserves non-tracking params", () => {
    const url =
      "https://example.com/search?q=hello&page=2&utm_source=newsletter";
    const cleaned = cleanLinkHref(url);
    expect(cleaned).toContain("q=hello");
    expect(cleaned).toContain("page=2");
    expect(cleaned).not.toContain("utm_source");
  });

  it("handles URL with only tracking params (strips all)", () => {
    const url = "https://example.com/page?utm_source=test&fbclid=abc&gclid=xyz";
    const cleaned = cleanLinkHref(url);
    expect(cleaned).toBe("https://example.com/page");
  });

  it("handles URL with no params (returns unchanged)", () => {
    const url = "https://example.com/page";
    expect(cleanLinkHref(url)).toBe(url);
  });

  it("handles URL with hash but no tracking params", () => {
    const url = "https://example.com/page#section";
    expect(cleanLinkHref(url)).toBe(url);
  });

  it("handles invalid URL (returns unchanged)", () => {
    const invalid = "not a url at all";
    expect(cleanLinkHref(invalid)).toBe(invalid);
  });

  it("handles empty string (returns unchanged)", () => {
    expect(cleanLinkHref("")).toBe("");
  });

  it("handles non-http URL like mailto: (returns unchanged)", () => {
    const mailto = "mailto:user@example.com";
    expect(cleanLinkHref(mailto)).toBe(mailto);
  });

  it("handles non-http URL like tel: (returns unchanged)", () => {
    const tel = "tel:+1234567890";
    expect(cleanLinkHref(tel)).toBe(tel);
  });

  it("handles non-http URL like javascript: (returns unchanged)", () => {
    const js = "javascript:void(0)";
    expect(cleanLinkHref(js)).toBe(js);
  });

  it("preserves path and fragment when stripping params", () => {
    const url =
      "https://example.com/path/to/page?utm_source=test&keep=yes#heading";
    const cleaned = cleanLinkHref(url);
    expect(cleaned).toContain("/path/to/page");
    expect(cleaned).toContain("keep=yes");
    expect(cleaned).toContain("#heading");
    expect(cleaned).not.toContain("utm_source");
  });

  it("handles http:// protocol the same as https://", () => {
    const url = "http://example.com/?utm_source=test";
    const cleaned = cleanLinkHref(url);
    expect(cleaned).toBe("http://example.com/");
  });

  it("removes mixed tracking params from multiple vendors", () => {
    const url =
      "https://example.com/?utm_source=google&fbclid=fb1&msclkid=ms1&twclid=tw1&_ga=ga1";
    const cleaned = cleanLinkHref(url);
    expect(cleaned).toBe("https://example.com/");
  });
});

describe("sanitizeLinkNode", () => {
  it("sets target=_blank on anchor element", () => {
    const a = document.createElement("a");
    a.setAttribute("href", "https://example.com");
    sanitizeLinkNode(a);
    expect(a.getAttribute("target")).toBe("_blank");
  });

  it("sets rel=noopener noreferrer on anchor element", () => {
    const a = document.createElement("a");
    a.setAttribute("href", "https://example.com");
    sanitizeLinkNode(a);
    expect(a.getAttribute("rel")).toBe("noopener noreferrer");
  });

  it("strips tracking params from href", () => {
    const a = document.createElement("a");
    a.setAttribute("href", "https://example.com?utm_source=test&keep=yes");
    sanitizeLinkNode(a);
    const href = a.getAttribute("href")!;
    expect(href).not.toContain("utm_source");
    expect(href).toContain("keep=yes");
  });

  it("skips non-A elements (does not modify)", () => {
    const div = document.createElement("div");
    div.setAttribute("href", "https://example.com?utm_source=test");
    sanitizeLinkNode(div);
    expect(div.getAttribute("target")).toBeNull();
    expect(div.getAttribute("rel")).toBeNull();
    expect(div.getAttribute("href")).toBe(
      "https://example.com?utm_source=test"
    );
  });

  it("skips anchor without href", () => {
    const a = document.createElement("a");
    sanitizeLinkNode(a);
    expect(a.getAttribute("target")).toBeNull();
    expect(a.getAttribute("rel")).toBeNull();
  });

  it("respects stripTracking=false parameter", () => {
    const a = document.createElement("a");
    a.setAttribute("href", "https://example.com?utm_source=test");
    sanitizeLinkNode(a, false);
    expect(a.getAttribute("target")).toBe("_blank");
    expect(a.getAttribute("rel")).toBe("noopener noreferrer");
    expect(a.getAttribute("href")).toContain("utm_source");
  });

  it("handles mailto: href (keeps href unchanged, still sets target/rel)", () => {
    const a = document.createElement("a");
    a.setAttribute("href", "mailto:user@example.com");
    sanitizeLinkNode(a);
    expect(a.getAttribute("target")).toBe("_blank");
    expect(a.getAttribute("href")).toBe("mailto:user@example.com");
  });
});
