"""Checks the built site: every page rendered (no Liquid left), in the
language of its section (ru/ or not), has a title and a description, links to pages and anchors that exist, and each page
with a translation links to it and back (hreflang).

Usage: python3 tools/site/check.py <built site dir>
"""
import html.parser
import os
import pathlib
import sys
import urllib.parse

BASE = "/fbrescue/"
SITE = "https://formeo.github.io/fbrescue/"


class Page(html.parser.HTMLParser):
    def __init__(self):
        super().__init__()
        self.links, self.alternates, self.title, self.description, self.lang = [], {}, "", "", ""
        self._in_title = False

    def handle_starttag(self, tag, attrs):
        a = dict(attrs)
        if tag == "html":
            self.lang = a.get("lang", "")
        if tag == "a" and "href" in a:
            self.links.append(a["href"])
        if tag == "link" and a.get("rel") == "alternate" and "hreflang" in a:
            self.alternates[a["hreflang"]] = a["href"]
        if tag == "meta" and a.get("name") == "description":
            self.description = a.get("content", "")
        self._in_title = tag == "title"

    def handle_data(self, data):
        if self._in_title:
            self.title += data

    def handle_endtag(self, tag):
        self._in_title = False


def target(site, href):
    """The file a site-internal link points to, or None for external links."""
    if href.startswith(SITE):
        href = BASE + href[len(SITE):]
    if not href.startswith(BASE):
        return None
    path = urllib.parse.urlparse(href).path[len(BASE):]
    f = site / path
    return f / "index.html" if path == "" or path.endswith("/") else f


def main(site):
    site = pathlib.Path(site)
    errors, pages = [], {}
    for f in sorted(site.rglob("*.html")):
        text = f.read_text(encoding="utf-8")
        if text.startswith("google-site-verification:"):
            continue  # Search Console ownership file, served as is
        p = Page()
        p.feed(text)
        pages[f] = p
        rel = f.relative_to(site)
        if "{%" in text or "{{" in text:
            errors.append(f"{rel}: Liquid left unrendered")
        if not p.title.strip() or not p.description.strip():
            errors.append(f"{rel}: no title or description")
        if not p.alternates:
            errors.append(f"{rel}: no hreflang links (every page has a translation; is the front matter valid?)")
        if p.lang != p_lang(f, site):
            errors.append(f"{rel}: <html lang> is {p.lang!r}, the page is in {p_lang(f, site)}")
    for f, p in pages.items():
        rel = f.relative_to(site)
        for href in p.links:
            if href.startswith(("http://", "https://")) and not href.startswith(SITE):
                continue
            full = urllib.parse.urljoin(SITE + str(rel.parent) + "/", href) if not href.startswith("/") else href
            t = target(site, full)
            if t is not None and not t.exists():
                errors.append(f"{rel}: broken link {href}")
        for lang, href in p.alternates.items():
            t = target(site, href)
            if t is None or not t.exists():
                errors.append(f"{rel}: hreflang {lang} points to a missing page {href}")
            elif pages[t].alternates.get(p_lang(f, site)) is None:
                errors.append(f"{rel}: {href} does not link back")
    for need in ["index.html", "ru/index.html", "errors/index.html", "sitemap.xml", "robots.txt"]:
        if not (site / need).exists():
            errors.append(f"missing {need}")
    for e in errors:
        print(f"::error::{e}" if os.environ.get("GITHUB_ACTIONS") else e)
    print(f"{len(pages)} pages, {len(errors)} problems")
    return 1 if errors else 0


def p_lang(f, site):
    return "ru" if f.relative_to(site).parts[0] == "ru" else "en"


if __name__ == "__main__":
    sys.exit(main(sys.argv[1]))
