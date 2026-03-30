#!/usr/bin/env python
"""
Indeed job scraper.

Reads search config from --config <yaml path>.
Outputs a JSON array of ScrapedJob objects to stdout.
All errors go to stderr so they don't corrupt the JSON output.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

import yaml
from playwright.sync_api import sync_playwright

sys.path.insert(0, str(Path(__file__).parent.parent))
from shared.browser import get_browser_context, human_delay
from shared.models import ScrapedJob


def build_search_url(query: str, location: str, remote: bool = False) -> str:
    """Construct an Indeed jobs search URL."""
    import urllib.parse
    base = "https://www.indeed.com/jobs?"
    params = {"q": query, "l": location, "fromage": "1"}  # past 1 day
    if remote:
        params["remotejob"] = "032b3046-06a3-4876-8dfd-474eb5e7ed11"
    return base + urllib.parse.urlencode(params)


def scrape(config: dict) -> list[ScrapedJob]:
    """
    Scrape Indeed job listings.
    TODO: implement search navigation, job card extraction, pagination.
    Indeed uses heavy JS rendering — use Playwright and wait for job cards to load.
    """
    jobs: list[ScrapedJob] = []

    with sync_playwright() as p:
        context = get_browser_context(p, headless=True)
        page = context.new_page()

        # TODO:
        # 1. Navigate to search URLs for each target role/location
        # 2. Wait for job card list to render
        # 3. Extract: title, company, location, salary snippet, job key (external_id)
        # 4. Click into each card to get full description
        # 5. Detect "Apply on Indeed" (Easy Apply) vs external
        # 6. Paginate
        # Note: Indeed uses a "jobKeys" parameter — use that as external_id

        context.close()

    return jobs


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--config", required=True, help="Path to config.yaml")
    args = parser.parse_args()

    try:
        with open(args.config) as f:
            config = yaml.safe_load(f)
        results = scrape(config)
        print(json.dumps([j.model_dump() for j in results]))
    except Exception as e:
        print(f"ERROR: {e}", file=sys.stderr)
        sys.exit(1)
