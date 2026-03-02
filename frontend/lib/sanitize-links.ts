const TRACKING_PARAMS = new Set([
  // Google / UTM
  "utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content",
  "utm_id", "utm_source_platform", "utm_creative_format", "utm_marketing_tactic",
  "gclid", "gbraid", "wbraid", "dclid",
  "_ga", "_gl",
  // Facebook / Meta
  "fbclid", "fbc", "__cft__", "__tn__",
  // Twitter / X
  "twclid",
  // TikTok
  "ttclid",
  // LinkedIn
  "li_fat_id", "trk",
  // Microsoft
  "msclkid",
  // Mailchimp
  "mc_cid", "mc_eid",
  // HubSpot
  "_hsenc", "_hsmi",
  // Klaviyo
  "_ke",
  // Marketo
  "mkt_tok",
  // Spotify / YouTube
  "si", "feature", "pp", "ab_channel",
  // Instagram
  "igshid",
  // Snapchat
  "sfnsn",
  // Ometria
  "oly_enc_id", "oly_anon_id",
  // Vero
  "vero_id",
  // BounceX
  "_bta_tid", "_bta_c",
  // Drip
  "__s",
  // Adobe
  "s_cid",
  // Yandex
  "yclid",
  // OpenStat
  "_openstat",
  // General referral
  "ref", "ref_src", "ref_url",
  // Impact Radius
  "irclickid",
]);

export function cleanLinkHref(href: string): string {
  try {
    const url = new URL(href);
    if (url.protocol !== "http:" && url.protocol !== "https:") return href;
    let changed = false;
    for (const key of [...url.searchParams.keys()]) {
      if (TRACKING_PARAMS.has(key)) {
        url.searchParams.delete(key);
        changed = true;
      }
    }
    return changed ? url.toString() : href;
  } catch {
    return href;
  }
}

export function sanitizeLinkNode(node: Element, stripTracking = true): void {
  if (node.tagName !== "A") return;
  const href = node.getAttribute("href");
  if (!href) return;
  node.setAttribute("target", "_blank");
  node.setAttribute("rel", "noopener noreferrer");
  if (stripTracking) {
    node.setAttribute("href", cleanLinkHref(href));
  }
}
