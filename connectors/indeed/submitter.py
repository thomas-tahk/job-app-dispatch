#!/usr/bin/env python
"""
Indeed job submitter.

Reads a SubmitRequest JSON from stdin.
Outputs a SubmitResult JSON to stdout.
All errors go to stderr.
"""

from __future__ import annotations

import json
import sys
from pathlib import Path

from playwright.sync_api import sync_playwright

sys.path.insert(0, str(Path(__file__).parent.parent))
from shared.browser import get_browser_context, human_delay
from shared.models import SubmitRequest, SubmitResult


def submit_indeed_apply(page, req: SubmitRequest) -> SubmitResult:
    """
    Attempt Indeed's "Apply on Indeed" flow.
    Falls back to manual URL for complex multi-step forms or external redirects.
    TODO: implement.
    """
    return SubmitResult(
        success=False,
        failure_reason="not yet implemented",
        manual_url=req.job.apply_url,
    )


def submit(req: SubmitRequest) -> SubmitResult:
    if not req.job.is_easy_apply:
        return SubmitResult(
            success=False,
            failure_reason="external application — manual submission required",
            manual_url=req.job.apply_url,
        )

    with sync_playwright() as p:
        context = get_browser_context(p, headless=True)
        page = context.new_page()
        try:
            result = submit_indeed_apply(page, req)
        except Exception as e:
            result = SubmitResult(
                success=False,
                failure_reason=str(e),
                manual_url=req.job.apply_url,
            )
        finally:
            context.close()

    return result


if __name__ == "__main__":
    try:
        raw = sys.stdin.read()
        req = SubmitRequest.model_validate_json(raw)
        result = submit(req)
        print(result.model_dump_json())
    except Exception as e:
        print(json.dumps({"success": False, "failure_reason": str(e)}))
        sys.exit(1)
