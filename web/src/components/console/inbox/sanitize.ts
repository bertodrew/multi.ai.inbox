// Email HTML is untrusted. This module rebuilds a body from an allowlist of
// tags and attributes, drops everything that can run or phone home (scripts,
// styles, frames, forms, event handlers, url() backgrounds) and replaces each
// remote image with a placeholder until the reader asks for images.
//
// It runs in the browser only: DOMParser exists there, and the parsed document
// is inert, so nothing loads while we clean it.

export type BlockedImage = { src: string; alt: string };

export type SanitizedBody = {
  html: string;
  /** Remote images found in the body, in document order. */
  blockedImages: BlockedImage[];
  /** Images dropped outright: inline cid references and tracking pixels. */
  droppedImages: number;
};

const ALLOWED_TAGS = new Set([
  "a", "abbr", "b", "blockquote", "br", "caption", "center", "cite", "code", "col", "colgroup",
  "dd", "div", "dl", "dt", "em", "figcaption", "figure", "h1", "h2", "h3", "h4", "h5", "h6",
  "hr", "i", "img", "li", "ol", "p", "pre", "q", "s", "small", "span", "strong", "sub", "sup",
  "table", "tbody", "td", "tfoot", "th", "thead", "tr", "u", "ul",
]);

/** Removed with their content: their children are never safe to keep. */
const DROP_TAGS = new Set([
  "applet", "audio", "base", "button", "canvas", "embed", "form", "frame", "frameset", "head",
  "iframe", "input", "link", "math", "meta", "noscript", "object", "option", "script", "select",
  "source", "style", "svg", "template", "textarea", "title", "track", "video",
]);

const ALLOWED_ATTRS: Record<string, readonly string[]> = {
  a: ["href", "title"],
  img: ["src", "alt", "title", "width", "height"],
  td: ["colspan", "rowspan"],
  th: ["colspan", "rowspan"],
  col: ["span"],
  colgroup: ["span"],
};

const HTTP_URL = /^https?:\/\//i;
const MAILTO_URL = /^mailto:/i;

function safeLink(href: string): string | null {
  const value = (href ?? "").trim();
  if (HTTP_URL.test(value) || MAILTO_URL.test(value)) return value;
  return null;
}

function imageHost(src: string): string {
  try {
    return new URL(src).host;
  } catch {
    return "remote host";
  }
}

function unwrap(element: Element): void {
  const parent = element.parentNode;
  if (!parent) return;
  while (element.firstChild) parent.insertBefore(element.firstChild, element);
  parent.removeChild(element);
}

function placeholder(doc: Document, image: BlockedImage): HTMLElement {
  const span = doc.createElement("span");
  span.setAttribute("data-mai-image", "blocked");
  span.setAttribute("title", image.src);
  span.textContent = image.alt ? `Remote image blocked: ${image.alt}` : `Remote image blocked (${imageHost(image.src)})`;
  return span;
}

/**
 * Returns HTML that is safe to render with dangerouslySetInnerHTML. When
 * `showImages` is false every http(s) image becomes a placeholder and its
 * source is reported back so the reader can list or allow it.
 */
export function sanitizeEmailHtml(raw: string, options: { showImages: boolean }): SanitizedBody {
  const blockedImages: BlockedImage[] = [];
  let droppedImages = 0;
  if (typeof DOMParser === "undefined") return { html: "", blockedImages, droppedImages };

  const doc = new DOMParser().parseFromString(raw, "text/html");
  // document order keeps unwrapping parents before their children are visited
  for (const element of Array.from(doc.body.querySelectorAll("*"))) {
    if (!element.isConnected) continue;
    const tag = element.tagName.toLowerCase();

    if (DROP_TAGS.has(tag)) {
      element.remove();
      continue;
    }
    if (!ALLOWED_TAGS.has(tag)) {
      unwrap(element);
      continue;
    }

    const allowed = ALLOWED_ATTRS[tag] ?? [];
    for (const attribute of Array.from(element.attributes)) {
      if (!allowed.includes(attribute.name.toLowerCase())) element.removeAttribute(attribute.name);
    }

    if (tag === "a") {
      const href = safeLink(element.getAttribute("href") ?? "");
      if (href) {
        element.setAttribute("href", href);
        element.setAttribute("target", "_blank");
        element.setAttribute("rel", "noopener noreferrer nofollow");
      } else {
        element.removeAttribute("href");
      }
    }

    if (tag === "img") {
      const src = (element.getAttribute("src") ?? "").trim();
      const alt = (element.getAttribute("alt") ?? "").trim();
      const width = Number(element.getAttribute("width") ?? "");
      const height = Number(element.getAttribute("height") ?? "");
      const tracking = (width > 0 && width <= 2) || (height > 0 && height <= 2);

      // Inline images live in the raw message, not behind the API, a cid
      // reference has nothing to resolve against here, and a 1px image is a
      // tracking pixel: none of them is worth a request.
      if (tracking || /^cid:/i.test(src) || !HTTP_URL.test(src)) {
        element.remove();
        droppedImages += 1;
        continue;
      }
      if (!options.showImages) {
        blockedImages.push({ src, alt });
        element.replaceWith(placeholder(doc, { src, alt }));
        continue;
      }
      blockedImages.push({ src, alt });
      element.setAttribute("loading", "lazy");
      element.setAttribute("decoding", "async");
      element.setAttribute("referrerpolicy", "no-referrer");
    }
  }

  return { html: doc.body.innerHTML, blockedImages, droppedImages };
}

/** True when the body has any element at all, so we can skip empty shells. */
export function hasVisibleContent(html: string): boolean {
  if (typeof DOMParser === "undefined") return html.trim() !== "";
  const doc = new DOMParser().parseFromString(html, "text/html");
  return (doc.body.textContent ?? "").trim() !== "" || doc.body.querySelector("img,hr,table") !== null;
}
