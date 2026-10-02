"""Real-browser regression for popup interception and redirect base URLs."""

from pathlib import Path

import pytest
from core.url_access import Download
from services.document.fetcher import URLFetcher


@pytest.mark.integration
async def test_browser_redirect_base_and_popup_first_request_are_guarded(monkeypatch):
    playwright = pytest.importorskip("playwright.async_api")
    async with playwright.async_playwright() as driver:
        if not Path(driver.chromium.executable_path).exists():
            pytest.skip("Playwright Chromium is not installed")
    seen = []
    body = b"""<html><body>main<script src="app.js"></script>
        <script>window.open('http://127.0.0.1/private', '_blank');</script></body></html>"""

    async def download(url, timeout):
        seen.append(url)
        if url == "https://public.example/start":
            return Download(body, "https://public.example/docs/", "text/html", 200)
        if url == "https://public.example/docs/":
            return Download(body, url, "text/html", 200)
        if url == "https://public.example/docs/app.js":
            return Download(b"document.body.append('script-loaded');", url, "text/javascript", 200)
        raise ValueError("Blocked network target")

    monkeypatch.setattr("services.document.fetcher.download_url", download)
    fetcher = URLFetcher()
    try:
        result = await fetcher._fetch_with_playwright(
            "https://public.example/start", 10, True, 300, False
        )
    finally:
        await fetcher.close()
    assert "script-loaded" in result["text"]
    assert "https://public.example/docs/app.js" in seen
    assert "http://127.0.0.1/private" in seen  # Intercepted before popup's first network request.
    assert fetcher._browser is None and fetcher._playwright is None
