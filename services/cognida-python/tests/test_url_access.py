"""URL network boundary including DNS rebinding, redirects, and response limits."""

from types import SimpleNamespace

import httpx
import pytest
from core import url_access


@pytest.fixture(autouse=True)
def settings(monkeypatch):
    config = SimpleNamespace(url_allowed_hosts=[])
    monkeypatch.setattr(url_access, "get_settings", lambda: config)
    return config


def dns(monkeypatch, addresses):
    monkeypatch.setattr(
        url_access.socket,
        "getaddrinfo",
        lambda host, port, **kw: [(2, 1, 6, "", (ip, port)) for ip in addresses],
    )


@pytest.mark.parametrize(
    "url", ["file:///etc/passwd", "https://user:pass@example.com", "http://example.com"]
)
async def test_rejects_unsafe_schemes_and_credentials(url):
    with pytest.raises(ValueError):
        await url_access.resolve_url(url)


@pytest.mark.parametrize(
    "addresses",
    [
        ["127.0.0.1"],
        ["10.0.0.1"],
        ["169.254.169.254"],
        ["::1"],
        ["224.0.0.1"],
        ["8.8.8.8", "10.0.0.1"],
    ],
)
async def test_rejects_private_and_mixed_dns(monkeypatch, addresses):
    dns(monkeypatch, addresses)
    with pytest.raises(ValueError, match="non-public"):
        await url_access.resolve_url("https://example.com")


async def test_explicit_internal_exception(monkeypatch, settings):
    dns(monkeypatch, ["127.0.0.1"])
    settings.url_allowed_hosts = ["internal.example"]
    _, ip = await url_access.resolve_url("http://internal.example:8080/data")
    assert ip == "127.0.0.1"


def transport(monkeypatch, handler):
    client_class = httpx.AsyncClient
    monkeypatch.setattr(
        url_access.httpx,
        "AsyncClient",
        lambda **kwargs: client_class(transport=httpx.MockTransport(handler), **kwargs),
    )


async def test_connects_to_validated_ip_with_original_host_and_sni(monkeypatch):
    dns(monkeypatch, ["8.8.8.8"])
    seen = []

    def handler(request):
        seen.append(request)
        return httpx.Response(200, content=b"ok")

    transport(monkeypatch, handler)
    response = await url_access.download_url("https://example.com/data")
    assert response.content == b"ok"
    assert seen[0].url.host == "8.8.8.8"
    assert seen[0].headers["host"] == "example.com"
    assert seen[0].extensions["sni_hostname"] == "example.com"


async def test_redirect_to_private_target_is_rejected(monkeypatch):
    monkeypatch.setattr(
        url_access.socket,
        "getaddrinfo",
        lambda host, port, **kw: [
            (2, 1, 6, "", ("8.8.8.8" if host == "example.com" else "127.0.0.1", port))
        ],
    )
    seen = []

    def handler(request):
        seen.append(request)
        return httpx.Response(302, headers={"Location": "https://localhost/private"})

    transport(monkeypatch, handler)
    with pytest.raises(ValueError, match="non-public"):
        await url_access.download_url("https://example.com")
    assert len(seen) == 1


async def test_response_size_is_bounded(monkeypatch):
    dns(monkeypatch, ["8.8.8.8"])
    monkeypatch.setattr(url_access, "MAX_DOWNLOAD_BYTES", 2)
    transport(monkeypatch, lambda request: httpx.Response(200, content=b"too big"))
    with pytest.raises(ValueError, match="maximum download size"):
        await url_access.download_url("https://example.com")
