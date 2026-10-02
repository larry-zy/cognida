"""Validated, bounded URL downloads with DNS addresses pinned to each request."""

import asyncio
import ipaddress
import socket
from dataclasses import dataclass
from urllib.parse import urljoin, urlsplit

import httpx
from config import get_settings

MAX_DOWNLOAD_BYTES = 50 * 1024 * 1024
MAX_REDIRECTS = 5


@dataclass
class Download:
    content: bytes
    url: str
    content_type: str
    status_code: int


async def resolve_url(url: str) -> tuple[httpx.URL, str]:
    parsed = urlsplit(url)
    host = (parsed.hostname or "").rstrip(".").lower()
    allowed = {item.rstrip(".").lower() for item in get_settings().url_allowed_hosts}
    if not host or parsed.username or parsed.password or parsed.scheme not in {"http", "https"}:
        raise ValueError("URL must be HTTP(S) without embedded credentials")
    if parsed.scheme != "https" and host not in allowed:
        raise ValueError("Public URLs must use HTTPS")
    port = parsed.port or (443 if parsed.scheme == "https" else 80)
    addresses = await asyncio.to_thread(socket.getaddrinfo, host, port, type=socket.SOCK_STREAM)
    ips = list(dict.fromkeys(item[4][0] for item in addresses))
    if not ips:
        raise ValueError("URL hostname has no addresses")
    # Reject mixed answers as well; connection uses the validated address, never resolves again.
    parsed_ips = [ipaddress.ip_address(ip) for ip in ips]
    if host not in allowed and any(
        not ip.is_global or ip.is_multicast or ip.is_reserved for ip in parsed_ips
    ):
        raise ValueError("URL resolves to a non-public network address")
    return httpx.URL(url), ips[0]


async def download_url(url: str, timeout: float = 30) -> Download:
    """Check every redirect and bound response size, timeout and proxy behavior."""
    if timeout <= 0 or timeout > 120:
        raise ValueError("URL timeout must be between 0 and 120 seconds")
    async with asyncio.timeout(timeout):
        async with httpx.AsyncClient(
            timeout=timeout,
            follow_redirects=False,
            trust_env=False,
            limits=httpx.Limits(max_keepalive_connections=0),
        ) as client:
            for _ in range(MAX_REDIRECTS + 1):
                original, address = await resolve_url(url)
                pinned = original.copy_with(host=address)
                # Host and TLS server name remain the original origin even though DNS is pinned.
                headers = {"Host": original.netloc.decode("ascii")}
                extensions = {"sni_hostname": original.host}
                async with client.stream(
                    "GET", pinned, headers=headers, extensions=extensions
                ) as response:
                    if response.status_code in {301, 302, 303, 307, 308}:
                        location = response.headers.get("location")
                        if not location:
                            raise ValueError("Redirect response is missing Location")
                        url = urljoin(str(original), location)
                        continue
                    response.raise_for_status()
                    chunks = []
                    size = 0
                    async for chunk in response.aiter_bytes():
                        size += len(chunk)
                        if size > MAX_DOWNLOAD_BYTES:
                            raise ValueError("URL response exceeds maximum download size")
                        chunks.append(chunk)
                    return Download(
                        b"".join(chunks),
                        str(original),
                        response.headers.get("content-type", ""),
                        response.status_code,
                    )
    raise ValueError("URL exceeded maximum redirect count")
