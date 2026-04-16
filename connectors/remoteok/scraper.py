#!/usr/bin/env python
"""
RemoteOK scraper — https://remoteok.com/api

Free, no auth required. Returns remote-only jobs worldwide.
Filters by dev/IT tags derived from config role keywords.
Outputs a JSON array of ScrapedJob to stdout.
"""

from __future__ import annotations

import argparse
import json
import sys
import time
from pathlib import Path

import requests
import yaml

sys.path.insert(0, str(Path(__file__).parent.parent))
from shared.models import ScrapedJob

# RemoteOK tag vocabulary that maps to dev/IT roles.
_DEV_TAGS = ["dev", "frontend", "backend", "fullstack", "devops", "engineering",
             "react", "python", "golang", "javascript", "typescript", "node"]
_IT_TAGS = ["sysadmin", "it", "support", "networking", "security", "helpdesk"]

_SESSION = requests.Session()
_SESSION.headers.update({"Accept": "application/json"})
_TIMEOUT = 15


def _fetch(tags: list[str]) -> list[dict]:
    tag_str = ",".join(tags)
    url = f"https://remoteok.com/api?tags={tag_str}"
    # RemoteOK requests a 1-second delay between calls.
    time.sleep(1)
    resp = _SESSION.get(url, timeout=_TIMEOUT)
    resp.raise_for_status()
    data = resp.json()
    # First element is metadata, not a job.
    return [item for item in data if isinstance(item, dict) and "id" in item]


def _parse_salary(item: dict) -> tuple[float, float, str]:
    """RemoteOK occasionally includes salary_min/salary_max (annual USD)."""
    lo = float(item.get("salary_min") or 0)
    hi = float(item.get("salary_max") or lo)
    if lo > 0:
        return lo / 2080, hi / 2080, f"${lo:,.0f}–${hi:,.0f}/yr"
    return 0.0, 0.0, ""


def scrape(config: dict) -> list[ScrapedJob]:
    dev_kws = config.get("targets", {}).get("role_keywords", {}).get("dev", [])
    it_kws = config.get("targets", {}).get("role_keywords", {}).get("it", [])
    want_it = bool(it_kws)

    tags = list(_DEV_TAGS)
    if want_it:
        tags += _IT_TAGS

    try:
        raw_jobs = _fetch(tags)
    except Exception as exc:
        print(f"ERROR: remoteok fetch failed: {exc}", file=sys.stderr)
        return []

    print(f"INFO: remoteok: {len(raw_jobs)} raw jobs fetched", file=sys.stderr)

    jobs = []
    for item in raw_jobs:
        sal_min, sal_max, sal_raw = _parse_salary(item)
        date_str = (item.get("date") or "")[:10]
        description = item.get("description") or ""
        jobs.append(ScrapedJob(
            external_id=str(item["id"]),
            source="remoteok",
            title=item.get("position") or item.get("title") or "",
            company=item.get("company") or "",
            location=item.get("location") or "Remote",
            is_remote=True,
            salary_raw=sal_raw,
            salary_min=sal_min,
            salary_max=sal_max,
            has_healthcare=False,  # RemoteOK doesn't expose benefits
            description=description[:4000],
            apply_url=item.get("url") or item.get("apply_url") or "",
            posted_at=date_str,
        ))
    return jobs


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--config", required=True)
    args = parser.parse_args()

    try:
        with open(args.config) as f:
            cfg = yaml.safe_load(f)
        results = scrape(cfg)
        print(json.dumps([j.model_dump() for j in results]))
    except Exception as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        sys.exit(1)
