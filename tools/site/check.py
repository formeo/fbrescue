"""Checks the built site: every page rendered (no Liquid left), in the
language of its section (ru/ or not), has a title and a description, links to pages and anchors that exist, and each page
with a translation links to it and back (hreflang).

Usage: python3 tools/site/check.py <built site dir>
"""
import html.parser
import json
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
        self.jsonld = []  # parsed structured data, or the error
        self.meta = {}  # og:* and twitter:* properties
        self._in_title = self._in_jsonld = False

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
        if tag == "meta" and (a.get("property") or a.get("name") or "").startswith(("og:", "twitter:")):
            self.meta[a.get("property") or a.get("name")] = a.get("content", "")
        self._in_title = tag == "title"
        if tag == "script" and a.get("type") == "application/ld+json":
            self._in_jsonld = True
            self.jsonld.append("")

    def handle_data(self, data):
        if self._in_title:
            self.title += data
        if self._in_jsonld:
            self.jsonld[-1] += data

    def handle_endtag(self, tag):
        self._in_title = False
        if tag == "script":
            self._in_jsonld = False


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
        if f.name == "404.html" and f.parent == site:
            continue  # not indexed, one page for both languages
        p = Page()
        p.feed(text)
        pages[f] = p
        rel = f.relative_to(site)
        if "{%" in text or "{{" in text:
            errors.append(f"{rel}: Liquid left unrendered")
        if not p.title.strip() or not p.description.strip():
            errors.append(f"{rel}: no title or description")
        elif len(p.title.strip()) > 70:
            errors.append(f"{rel}: title is {len(p.title.strip())} characters, keep it within 70")
        elif not 25 <= len(p.description) <= 160:
            # Bing Webmaster flags descriptions outside 25-160 characters.
            errors.append(f"{rel}: description is {len(p.description)} characters, keep it within 25-160")
        types = []
        for block in p.jsonld:
            try:
                types.append(json.loads(block).get("@type"))
            except ValueError as e:
                errors.append(f"{rel}: structured data is not valid JSON: {e}")
        expect = {"index.html": "SoftwareApplication", "ru/index.html": "SoftwareApplication",
                  "faq/index.html": "FAQPage", "ru/faq/index.html": "FAQPage"}.get(str(rel))
        if expect and expect not in types:
            errors.append(f"{rel}: no {expect} structured data")
        want_locale = "ru_RU" if p_lang(f, site) == "ru" else "en_US"
        if p.meta.get("og:locale") != want_locale:
            errors.append(f"{rel}: og:locale is {p.meta.get('og:locale')!r}, want {want_locale}")
        img = target(site, p.meta.get("og:image", ""))
        if img is None or not img.exists():
            errors.append(f"{rel}: og:image {p.meta.get('og:image')!r} is missing or not on the site")
        if p.alternates and "x-default" not in p.alternates:
            errors.append(f"{rel}: no hreflang x-default")
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
    sitemap = site / "sitemap.xml"
    if sitemap.exists() and "google" in sitemap.read_text(encoding="utf-8"):
        errors.append("sitemap.xml lists the Search Console verification file")
    for need in ["index.html", "ru/index.html", "errors/index.html", "sitemap.xml", "robots.txt", "404.html",
                 "favicon.svg", "favicon-48.png"]:
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
