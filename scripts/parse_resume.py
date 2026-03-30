#!/usr/bin/env python
"""
Resume parser — called by the Go watcher whenever a file is added/changed in materials/resumes/.

Usage:  python scripts/parse_resume.py <filepath>
Output: JSON object with keys: filename, role_type, parsed_text, skills, titles
Errors: printed to stderr; exit code 1
"""

from __future__ import annotations

import json
import sys
from pathlib import Path


def parse_docx(path: str) -> str:
    import docx
    doc = docx.Document(path)
    return "\n".join(p.text for p in doc.paragraphs if p.text.strip())


def parse_pdf(path: str) -> str:
    import pdfplumber
    with pdfplumber.open(path) as pdf:
        return "\n".join(page.extract_text() or "" for page in pdf.pages)


def extract_skills(text: str) -> list[str]:
    """
    Extract skill keywords from resume text.
    TODO: implement — options include keyword list matching or a lightweight NLP pass.
    """
    return []


def extract_titles(text: str) -> list[str]:
    """
    Extract job titles from the experience section.
    TODO: implement — look for lines following 'Experience' header pattern.
    """
    return []


def classify_role_type(filename: str, text: str) -> str:
    """
    Determine whether this resume targets dev or IT roles.
    Uses filename as the primary signal (e.g. 'resume_dev.pdf' → 'dev').
    Falls back to keyword scan of the text.
    """
    lower_name = Path(filename).stem.lower()
    if "dev" in lower_name or "software" in lower_name or "engineer" in lower_name:
        return "dev"
    if "it" in lower_name or "tech" in lower_name or "support" in lower_name or "desk" in lower_name:
        return "it"
    # Fallback: scan text
    dev_signals = ["software", "developer", "engineer", "frontend", "backend", "fullstack"]
    it_signals  = ["help desk", "service desk", "technician", "it support", "desktop support"]
    lower_text  = text.lower()
    dev_hits = sum(1 for kw in dev_signals if kw in lower_text)
    it_hits  = sum(1 for kw in it_signals  if kw in lower_text)
    return "it" if it_hits > dev_hits else "dev"


if __name__ == "__main__":
    if len(sys.argv) < 2:
        print(json.dumps({"error": "filepath argument required"}), file=sys.stderr)
        sys.exit(1)

    path = sys.argv[1]
    ext  = Path(path).suffix.lower()

    try:
        if ext == ".docx":
            text = parse_docx(path)
        elif ext == ".pdf":
            text = parse_pdf(path)
        else:
            print(json.dumps({"error": f"unsupported file type: {ext}"}), file=sys.stderr)
            sys.exit(1)
    except Exception as e:
        print(json.dumps({"error": str(e)}), file=sys.stderr)
        sys.exit(1)

    result = {
        "filename":    Path(path).name,
        "role_type":   classify_role_type(path, text),
        "parsed_text": text,
        "skills":      extract_skills(text),
        "titles":      extract_titles(text),
    }
    print(json.dumps(result))
