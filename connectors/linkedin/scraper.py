#!/usr/bin/env python
"""
LinkedIn job scraper.

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


def build_search_url(keywords: str, location: str, remote: bool = False) -> str:
    """Construct a LinkedIn jobs search URL."""
    import urllib.parse
    base = "https://www.linkedin.com/jobs/search/?"
    params = {
        "keywords": keywords,
        "location": location,
        "f_TPR": "r86400",  # past 24 hours
    }
    if remote:
        params["f_WT"] = "2"  # remote filter
    return base + urllib.parse.urlencode(params)


def parse_salary(salary_text: str) -> tuple[float, float]:
    """
    Parse a salary string like '$20/hr', '$50,000 - $70,000/yr', etc.
    Returns (min_hourly, max_hourly). Returns (0, 0) if unparseable.
    TODO: implement robust salary parsing.
    """
    return 0.0, 0.0


def scrape(config: dict) -> list[ScrapedJob]:
    """
    Scrape LinkedIn job listings.
    TODO: implement search navigation, job card extraction, pagination.
    """
    jobs: list[ScrapedJob] = []

    with sync_playwright() as p:
        context = get_browser_context(p, headless=True)
        page = context.new_page()

        # TODO:
        # 1. Load session cookies if available (to avoid repeated login)
        # 2. Navigate to search URLs for each target role/location combination
        # 3. Iterate job cards, extract: title, company, location, salary, description, URL
        # 4. Detect Easy Apply vs external
        # 5. Parse salary into salary_min/salary_max
        # 6. Detect remote/relocation from location string
        # 7. Paginate until no new results or max_pages reached

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
