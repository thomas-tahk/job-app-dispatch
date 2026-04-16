#!/usr/bin/env python
"""
ATS API scraper — Greenhouse, Ashby, Lever.

Reads portals.yaml (sibling of config.yaml) for the company list.
Calls ATS JSON APIs directly — no Playwright, no login required.
Outputs a JSON array of ScrapedJob objects to stdout.
All log/error output goes to stderr.
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from concurrent.futures import ThreadPoolExecutor, as_completed
from html.parser import HTMLParser
from pathlib import Path

import requests
import yaml

sys.path.insert(0, str(Path(__file__).parent.parent))
from shared.models import ScrapedJob


_SESSION = requests.Session()
_SESSION.headers.update({"Accept": "application/json", "User-Agent": "job-search/1.0"})
_TIMEOUT = 10  # seconds per request

# Salary: matches "$25/hr", "$80k/yr", "$100,000 - $150,000", "$80k - $120k", etc.
_SAL_RE = re.compile(
    r'\$([\d,]+(?:\.\d+)?)\s*([kK]?)'
    r'(?:\s*[-–—]\s*\$?\s*([\d,]+(?:\.\d+)?)\s*([kK]?))?'
    r'(?:\s*/\s*|\s+per\s+)?(hr|hour|yr|year|annual)?',
    re.IGNORECASE,
)
_HEALTHCARE_KEYWORDS = ("health", "medical", "dental", "vision", "hsa", "fsa", "insurance")


# ── Utilities ─────────────────────────────────────────────────────────────────

class _HTMLStripper(HTMLParser):
    def __init__(self):
        super().__init__()
        self._parts: list[str] = []

    def handle_data(self, data: str) -> None:
        self._parts.append(data)

    def get_text(self) -> str:
        return re.sub(r'\s+', ' ', " ".join(self._parts)).strip()


def _strip_html(html: str) -> str:
    p = _HTMLStripper()
    p.feed(html or "")
    return p.get_text()


def _parse_salary(text: str) -> tuple[float, float, str]:
    """Best-effort salary extraction. Returns (min_hourly, max_hourly, raw_snippet)."""
    m = _SAL_RE.search(text)
    if not m:
        return 0.0, 0.0, ""

    def _num(digits: str | None, k: str | None) -> float:
        if not digits:
            return 0.0
        v = float(digits.replace(",", ""))
        return v * 1000 if (k or "").lower() == "k" else v

    lo = _num(m.group(1), m.group(2))
    hi = _num(m.group(3), m.group(4)) if m.group(3) else lo
    unit = (m.group(5) or "").lower()

    if not lo:
        return 0.0, 0.0, ""

    if unit in ("hr", "hour"):
        # Sanity check: hourly wages are $5–$200
        if 5 <= lo <= 200:
            return lo, max(hi, lo), m.group(0)
    elif unit in ("yr", "year", "annual"):
        if lo >= 1000:
            return lo / 2080, max(hi, lo) / 2080, m.group(0)
    else:
        # No unit — infer from magnitude
        if lo >= 5000:        # annual: $80,000 or $80k
            return lo / 2080, max(hi, lo) / 2080, m.group(0)
        elif 5 <= lo <= 200:  # hourly: $25
            return lo, max(hi, lo), m.group(0)

    return 0.0, 0.0, ""


def _has_healthcare(text: str) -> bool:
    lower = text.lower()
    return any(kw in lower for kw in _HEALTHCARE_KEYWORDS)


# ── ATS-specific fetchers ──────────────────────────────────────────────────────

def _greenhouse(company_name: str, slug: str) -> list[ScrapedJob]:
    url = f"https://boards-api.greenhouse.io/v1/boards/{slug}/jobs?content=true"
    data = _SESSION.get(url, timeout=_TIMEOUT).json()
    jobs = []
    for item in data.get("jobs", []):
        desc = _strip_html(item.get("content", ""))
        sal_min, sal_max, sal_raw = _parse_salary(desc)
        offices = item.get("offices", [])
        location = ", ".join(o.get("name", "") for o in offices if o.get("name")) \
                   or item.get("location", {}).get("name", "")
        is_remote = "remote" in location.lower() or "remote" in item.get("title", "").lower()
        jobs.append(ScrapedJob(
            external_id=f"{slug}:{item['id']}",
            source=slug,
            title=item.get("title", ""),
            company=company_name,
            location=location,
            is_remote=is_remote,
            salary_raw=sal_raw,
            salary_min=sal_min,
            salary_max=sal_max,
            has_healthcare=_has_healthcare(desc),
            description=desc[:4000],
            apply_url=item.get("absolute_url", ""),
            posted_at=(item.get("updated_at") or "")[:10],
        ))
    return jobs


def _ashby(company_name: str, slug: str) -> list[ScrapedJob]:
    url = f"https://api.ashbyhq.com/posting-api/job-board/{slug}?includeCompensation=true"
    data = _SESSION.get(url, timeout=_TIMEOUT).json()
    jobs = []
    for item in data.get("jobPostings", []):
        desc = _strip_html(item.get("descriptionHtml") or item.get("description", ""))

        # Prefer structured compensation; fall back to text parsing.
        comp = item.get("compensation") or {}
        sal_min, sal_max, sal_raw = 0.0, 0.0, item.get("compensationTierSummary", "")
        interval = (comp.get("interval") or "").lower()
        lo = float(comp.get("min") or 0)
        hi = float(comp.get("max") or lo)
        if lo > 0:
            if interval == "year":
                sal_min, sal_max = lo / 2080, hi / 2080
            elif interval == "hour":
                sal_min, sal_max = lo, hi
        if sal_min == 0:
            sal_min, sal_max, sal_raw = _parse_salary(sal_raw or desc)

        location = item.get("locationName", "")
        is_remote = item.get("isRemote", False) or "remote" in location.lower()
        jobs.append(ScrapedJob(
            external_id=f"{slug}:{item['id']}",
            source=slug,
            title=item.get("title", ""),
            company=company_name,
            location=location,
            is_remote=is_remote,
            salary_raw=sal_raw,
            salary_min=sal_min,
            salary_max=sal_max,
            has_healthcare=_has_healthcare(desc),
            description=desc[:4000],
            apply_url=item.get("jobUrl", ""),
            posted_at=(item.get("publishedDate") or "")[:10],
        ))
    return jobs


def _lever(company_name: str, slug: str) -> list[ScrapedJob]:
    url = f"https://api.lever.co/v0/postings/{slug}?mode=json"
    items = _SESSION.get(url, timeout=_TIMEOUT).json()
    jobs = []
    for item in items:
        parts = [_strip_html(item.get("description", ""))]
        for lst in item.get("lists", []):
            parts.append(f"{lst.get('text', '')}: {_strip_html(lst.get('content', ''))}")
        desc = " ".join(filter(None, parts))

        sal_min, sal_max, sal_raw = _parse_salary(desc)
        cats = item.get("categories", {})
        all_locs = cats.get("allLocations") or []
        location = cats.get("location") or (all_locs[0] if all_locs else "")
        is_remote = "remote" in location.lower()
        jobs.append(ScrapedJob(
            external_id=f"{slug}:{item['id']}",
            source=slug,
            title=item.get("text", ""),
            company=company_name,
            location=location,
            is_remote=is_remote,
            salary_raw=sal_raw,
            salary_min=sal_min,
            salary_max=sal_max,
            has_healthcare=_has_healthcare(desc),
            description=desc[:4000],
            apply_url=item.get("hostedUrl", ""),
            posted_at="",
        ))
    return jobs


# ── Orchestration ─────────────────────────────────────────────────────────────

_FETCHERS = {"greenhouse": _greenhouse, "ashby": _ashby, "lever": _lever}


def _fetch_company(company: dict) -> list[ScrapedJob]:
    ats = company["ats"].lower()
    fetcher = _FETCHERS.get(ats)
    if not fetcher:
        print(f"WARNING: unknown ATS {ats!r} for {company['name']}", file=sys.stderr)
        return []
    return fetcher(company["name"], company["slug"])


def _title_relevant(title: str, dev_kws: list[str], it_kws: list[str]) -> bool:
    lower = title.lower()
    return any(kw.lower() in lower for kw in dev_kws + it_kws)


def scrape(config: dict, portals: dict) -> list[ScrapedJob]:
    companies = portals.get("companies") or []
    if not companies:
        print("WARNING: portals.yaml has no companies — nothing to scrape", file=sys.stderr)
        return []

    dev_kws = config.get("targets", {}).get("role_keywords", {}).get("dev", [])
    it_kws = config.get("targets", {}).get("role_keywords", {}).get("it", [])
    filter_enabled = bool(dev_kws or it_kws)

    results: list[ScrapedJob] = []
    with ThreadPoolExecutor(max_workers=5) as pool:
        futures = {pool.submit(_fetch_company, c): c["name"] for c in companies}
        for fut in as_completed(futures):
            name = futures[fut]
            try:
                jobs = fut.result()
                if filter_enabled:
                    jobs = [j for j in jobs if _title_relevant(j.title, dev_kws, it_kws)]
                results.extend(jobs)
                print(f"INFO: {name}: {len(jobs)} relevant jobs", file=sys.stderr)
            except Exception as exc:
                print(f"ERROR: {name}: {exc}", file=sys.stderr)

    return results


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--config", required=True, help="Path to config.yaml")
    args = parser.parse_args()

    config_path = Path(args.config)
    portals_path = config_path.parent / "portals.yaml"

    try:
        with open(config_path) as f:
            cfg = yaml.safe_load(f)
        if not portals_path.exists():
            print(f"WARNING: portals.yaml not found at {portals_path} — copy portals.example.yaml to get started", file=sys.stderr)
            print("[]")
            sys.exit(0)
        with open(portals_path) as f:
            portals = yaml.safe_load(f) or {}
        jobs = scrape(cfg, portals)
        print(json.dumps([j.model_dump() for j in jobs]))
    except Exception as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        sys.exit(1)
