"""Shared Playwright browser utilities used by all connectors."""

from __future__ import annotations

import random
import time

from playwright.sync_api import BrowserContext, Playwright


def get_browser_context(playwright: Playwright, headless: bool = True) -> BrowserContext:
    """
    Launch a Chromium browser context with anti-detection settings.
    headless=False is useful for debugging scraper issues.
    """
    browser = playwright.chromium.launch(headless=headless)
    context = browser.new_context(
        user_agent=(
            "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
            "AppleWebKit/537.36 (KHTML, like Gecko) "
            "Chrome/120.0.0.0 Safari/537.36"
        ),
        viewport={"width": 1280, "height": 800},
        locale="en-US",
        timezone_id="America/Denver",
    )
    # Mask webdriver flag
    context.add_init_script("Object.defineProperty(navigator, 'webdriver', {get: () => undefined})")
    return context


def human_delay(min_ms: int = 400, max_ms: int = 1200) -> None:
    """Sleep a random interval to mimic human reading/interaction pace."""
    time.sleep(random.uniform(min_ms / 1000, max_ms / 1000))


def random_mouse_move(page) -> None:
    """Move mouse to a random position — helps avoid bot detection on some platforms."""
    x = random.randint(200, 1000)
    y = random.randint(100, 600)
    page.mouse.move(x, y)
