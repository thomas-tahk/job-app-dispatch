#!/usr/bin/env python
"""
Arbeitnow scraper — https://www.arbeitnow.com/api/job-board-api

Free, no auth required. Broad global job board with strong remote coverage.
Filters by title keywords derived from config role keywords.
Fetches up to MAX_PAGES pages per run.
Outputs a JSON array of ScrapedJob to stdout.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

import requests
import yaml

sys.path.insert(0, str(Path(__file__).parent.parent))
from shared.models import ScrapedJob

_SESSION = requests.Session()
_SESSION.headers.update({"Accept": "application/json"})
_TIMEOUT = 15
_MAX_PAGES = 3   # 10 jobs/page → 30 jobs max per run; adjust as needed
_API_URL = "https://www.arbeitnow.com/api/job-board-api"


def _fetch_page(page: int) -> tuple[list[dict], bool]:
    """Returns (jobs, has_more)."""
    resp = _SESSION.get(_API_URL, params={"page": page}, timeout=_TIMEOUT)
    resp.raise_for_status()
    data = resp.json()
    jobs = data.get("data") or []
    meta = data.get("meta") or {}
    has_more = page < int(meta.get("last_page") or 1)
    return jobs, has_more


def _title_relevant(title: str, dev_kws: list[str], it_kws: list[str]) -> bool:
    lower = title.lower()
    return any(kw.lower() in lower for kw in dev_kws + it_kws)


def scrape(config: dict) -> list[ScrapedJob]:
    dev_kws = config.get("targets", {}).get("role_keywords", {}).get("dev", [])
    it_kws = config.get("targets", {}).get("role_keywords", {}).get("it", [])
    filter_enabled = bool(dev_kws or it_kws)

    all_raw: list[dict] = []
    for page in range(1, _MAX_PAGES + 1):
        try:
            items, has_more = _fetch_page(page)
            all_raw.extend(items)
            if not has_more:
                break
        except Exception as exc:
            print(f"ERROR: arbeitnow page {page}: {exc}", file=sys.stderr)
            break

    print(f"INFO: arbeitnow: {len(all_raw)} raw jobs fetched", file=sys.stderr)

    jobs = []
    for item in all_raw:
        title = item.get("title") or ""
        if filter_enabled and not _title_relevant(title, dev_kws, it_kws):
            continue

        description = item.get("description") or ""
        is_remote = bool(item.get("remote"))
        # created_at is Unix timestamp
        ts = item.get("created_at") or 0
        posted_at = ""
        if ts:
            from datetime import datetime, timezone
            posted_at = datetime.fromtimestamp(ts, tz=timezone.utc).strftime("%Y-%m-%d")

        jobs.append(ScrapedJob(
            external_id=item.get("slug") or "",
            source="arbeitnow",
            title=title,
            company=item.get("company_name") or "",
            location=item.get("location") or "",
            is_remote=is_remote,
            salary_raw="",
            salary_min=0.0,
            salary_max=0.0,
            has_healthcare=False,
            description=description[:4000],
            apply_url=item.get("url") or "",
            posted_at=posted_at,
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
