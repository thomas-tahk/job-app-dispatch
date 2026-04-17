#!/usr/bin/env python
"""
Fetch a single job from a URL.

Supports Greenhouse, Ashby, and Lever URLs natively via their JSON APIs.
Falls back to HTML extraction (JSON-LD / OpenGraph) for other job boards.
Outputs a single ScrapedJob JSON object to stdout.
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path

import requests
import yaml

sys.path.insert(0, str(Path(__file__).parent.parent))
from shared.models import ScrapedJob
from ats.scraper import _strip_html, _parse_salary, _has_healthcare

_SESSION = requests.Session()
_SESSION.headers.update({"Accept": "application/json", "User-Agent": "job-search/1.0"})
_TIMEOUT = 15

# ── URL detection ─────────────────────────────────────────────────────────────

_ATS_PATTERNS = [
    ("greenhouse", re.compile(
        r'https?://(?:boards|job-boards)\.greenhouse\.io/([^/?#]+)/jobs/(\d+)'
    )),
    ("ashby", re.compile(
        r'https?://jobs\.ashbyhq\.com/([^/?#]+)/([^/?#]+)'
    )),
    ("lever", re.compile(
        r'https?://jobs\.lever\.co/([^/?#]+)/([^/?#]+)'
    )),
]


def _detect_ats(url: str) -> tuple[str, str, str] | None:
    """Returns (platform, company_slug, job_id) or None."""
    for platform, pattern in _ATS_PATTERNS:
        m = pattern.match(url)
        if m:
            return platform, m.group(1), m.group(2)
    return None


# ── ATS fetchers ──────────────────────────────────────────────────────────────

def _fetch_greenhouse(slug: str, job_id: str) -> ScrapedJob:
    url = f"https://boards-api.greenhouse.io/v1/boards/{slug}/jobs/{job_id}?content=true"
    item = _SESSION.get(url, timeout=_TIMEOUT).json()
    desc = _strip_html(item.get("content", ""))
    sal_min, sal_max, sal_raw = _parse_salary(desc)
    offices = item.get("offices") or []
    location = ", ".join(o.get("name", "") for o in offices if o.get("name")) \
               or (item.get("location") or {}).get("name", "")
    is_remote = "remote" in location.lower() or "remote" in item.get("title", "").lower()
    return ScrapedJob(
        external_id=f"{slug}:{item['id']}",
        source=slug,
        title=item.get("title", ""),
        company=item.get("company", {}).get("name", slug),
        location=location,
        is_remote=is_remote,
        salary_raw=sal_raw, salary_min=sal_min, salary_max=sal_max,
        has_healthcare=_has_healthcare(desc),
        description=desc[:4000],
        apply_url=item.get("absolute_url", ""),
        posted_at=(item.get("updated_at") or "")[:10],
    )


def _fetch_ashby(slug: str, job_id: str) -> ScrapedJob:
    url = f"https://api.ashbyhq.com/posting-api/job-board/{slug}?includeCompensation=true"
    data = _SESSION.get(url, timeout=_TIMEOUT).json()
    postings = data.get("jobPostings") or []
    item = next((p for p in postings if p.get("id") == job_id), None)
    if not item:
        raise ValueError(f"job {job_id!r} not found in Ashby board {slug!r}")
    desc = _strip_html(item.get("descriptionHtml") or item.get("description", ""))
    comp = item.get("compensation") or {}
    sal_min, sal_max, sal_raw = 0.0, 0.0, item.get("compensationTierSummary", "")
    lo, hi = float(comp.get("min") or 0), float(comp.get("max") or 0)
    if lo > 0:
        interval = (comp.get("interval") or "").lower()
        sal_min = lo / 2080 if interval == "year" else lo
        sal_max = hi / 2080 if interval == "year" else hi
    if sal_min == 0:
        sal_min, sal_max, sal_raw = _parse_salary(sal_raw or desc)
    location = item.get("locationName", "")
    return ScrapedJob(
        external_id=f"{slug}:{item['id']}",
        source=slug,
        title=item.get("title", ""),
        company=item.get("teamName", slug),
        location=location,
        is_remote=item.get("isRemote", False) or "remote" in location.lower(),
        salary_raw=sal_raw, salary_min=sal_min, salary_max=sal_max,
        has_healthcare=_has_healthcare(desc),
        description=desc[:4000],
        apply_url=item.get("jobUrl", ""),
        posted_at=(item.get("publishedDate") or "")[:10],
    )


def _fetch_lever(slug: str, job_id: str) -> ScrapedJob:
    url = f"https://api.lever.co/v0/postings/{slug}/{job_id}"
    item = _SESSION.get(url, timeout=_TIMEOUT).json()
    parts = [_strip_html(item.get("description", ""))]
    for lst in item.get("lists") or []:
        parts.append(f"{lst.get('text', '')}: {_strip_html(lst.get('content', ''))}")
    desc = " ".join(filter(None, parts))
    sal_min, sal_max, sal_raw = _parse_salary(desc)
    cats = item.get("categories") or {}
    location = cats.get("location", "")
    return ScrapedJob(
        external_id=f"{slug}:{item['id']}",
        source=slug,
        title=item.get("text", ""),
        company=item.get("company", slug),
        location=location,
        is_remote="remote" in location.lower(),
        salary_raw=sal_raw, salary_min=sal_min, salary_max=sal_max,
        has_healthcare=_has_healthcare(desc),
        description=desc[:4000],
        apply_url=item.get("hostedUrl", ""),
        posted_at="",
    )


# ── HTML fallback ─────────────────────────────────────────────────────────────

def _fetch_html(url: str) -> ScrapedJob:
    """
    General-purpose fallback for non-ATS URLs.
    Tries JSON-LD JobPosting schema first, then OpenGraph tags, then <title>.
    """
    resp = requests.get(url, timeout=_TIMEOUT, headers={
        "User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36"
    })
    resp.raise_for_status()
    html = resp.text

    # 1. JSON-LD JobPosting
    for script in re.findall(r'<script[^>]*type=["\']application/ld\+json["\'][^>]*>(.*?)</script>',
                              html, re.DOTALL | re.IGNORECASE):
        try:
            data = json.loads(script)
            if isinstance(data, list):
                data = next((d for d in data if d.get("@type") == "JobPosting"), {})
            if data.get("@type") == "JobPosting":
                desc = _strip_html(data.get("description", ""))
                sal_min, sal_max, sal_raw = _parse_salary(desc)
                org = data.get("hiringOrganization") or {}
                company = org.get("name", "") if isinstance(org, dict) else str(org)
                location = ""
                loc = data.get("jobLocation")
                if isinstance(loc, dict):
                    addr = loc.get("address") or {}
                    location = ", ".join(filter(None, [
                        addr.get("addressLocality", ""),
                        addr.get("addressRegion", ""),
                    ])) if isinstance(addr, dict) else str(addr)
                return ScrapedJob(
                    external_id=url,
                    source="manual",
                    title=data.get("title", ""),
                    company=company,
                    location=location,
                    is_remote="remote" in (data.get("jobLocationType") or "").lower(),
                    salary_raw=sal_raw, salary_min=sal_min, salary_max=sal_max,
                    has_healthcare=_has_healthcare(desc),
                    description=desc[:4000],
                    apply_url=url,
                    posted_at=(data.get("datePosted") or "")[:10],
                )
        except (json.JSONDecodeError, AttributeError):
            continue

    # 2. OpenGraph fallback
    og_title = re.search(r'<meta[^>]+property=["\']og:title["\'][^>]+content=["\']([^"\']+)',
                         html, re.IGNORECASE)
    og_desc = re.search(r'<meta[^>]+property=["\']og:description["\'][^>]+content=["\']([^"\']+)',
                        html, re.IGNORECASE)
    title = og_title.group(1) if og_title else ""
    desc = og_desc.group(1) if og_desc else ""

    # 3. <title> last resort
    if not title:
        t = re.search(r'<title[^>]*>([^<]+)</title>', html, re.IGNORECASE)
        title = t.group(1).strip() if t else url

    sal_min, sal_max, sal_raw = _parse_salary(desc)
    return ScrapedJob(
        external_id=url,
        source="manual",
        title=title,
        company="",
        location="",
        is_remote=False,
        salary_raw=sal_raw, salary_min=sal_min, salary_max=sal_max,
        has_healthcare=_has_healthcare(desc),
        description=desc[:4000],
        apply_url=url,
        posted_at="",
    )


# ── Entry point ───────────────────────────────────────────────────────────────

def fetch(url: str) -> ScrapedJob:
    ats = _detect_ats(url)
    if ats:
        platform, slug, job_id = ats
        if platform == "greenhouse":
            return _fetch_greenhouse(slug, job_id)
        elif platform == "ashby":
            return _fetch_ashby(slug, job_id)
        elif platform == "lever":
            return _fetch_lever(slug, job_id)
    return _fetch_html(url)


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--url", required=True, help="Job posting URL")
    parser.add_argument("--config", required=True, help="Path to config.yaml")
    args = parser.parse_args()

    try:
        job = fetch(args.url)
        print(json.dumps(job.model_dump()))
    except Exception as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        sys.exit(1)
