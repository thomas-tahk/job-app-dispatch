from __future__ import annotations

from typing import Optional
from pydantic import BaseModel


class ScrapedJob(BaseModel):
    """Raw job data returned by a scraper script to Go via stdout JSON."""
    external_id: str
    title: str
    company: str
    location: str
    is_remote: bool = False
    salary_raw: str = ""
    salary_min: float = 0.0
    salary_max: float = 0.0
    has_healthcare: bool = False
    benefits: str = ""
    description: str = ""
    apply_url: str = ""
    is_easy_apply: bool = False
    posted_at: str = ""


class SubmitRequest(BaseModel):
    """Passed to a submitter script from Go via stdin JSON."""
    job: ScrapedJob
    cover_letter: str
    resume_file: str       # absolute path to the PDF
    linkedin_url: str
    github_url: str


class SubmitResult(BaseModel):
    """Returned by a submitter script to Go via stdout JSON."""
    success: bool
    failure_reason: Optional[str] = None
    manual_url: Optional[str] = None  # set when the form is too complex to automate
