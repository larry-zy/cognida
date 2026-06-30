"""URL内容获取模块。"""

from typing import Any
from dataclasses import dataclass

from core import get_logger
from bs4 import BeautifulSoup

# Playwright 是可选依赖
try:
    from playwright.async_api import async_playwright, Browser
    PLAYWRIGHT_AVAILABLE = True
except ImportError:
    PLAYWRIGHT_AVAILABLE = False
    Browser = None


@dataclass
class FetchResult:
    """URL获取结果。"""

    text: str
    html: str
    title: str
    links: list[str]
    screenshot: bytes | None = None


class URLFetcher:
    """URL内容获取器。

    支持：
    - 静态HTML获取
    - 动态JS渲染
    - 截图功能
    """

    def __init__(self) -> None:
        self._logger = get_logger(__name__)
        self._browser: Browser | None = None

    async def _get_browser(self) -> Browser:
        """获取浏览器实例。"""
        if not PLAYWRIGHT_AVAILABLE:
            raise RuntimeError("Playwright is not installed. Install it with: pip install playwright")
        if self._browser is None:
            playwright = await async_playwright().start()
            self._browser = await playwright.chromium.launch()
        return self._browser

    async def close(self) -> None:
        """关闭浏览器。"""
        if self._browser:
            await self._browser.close()
            self._browser = None

    async def fetch(
        self,
        url: str,
        timeout: int = 30,
        wait_for_javascript: bool = False,
        wait_time: int = 1000,
        screenshot: bool = False,
    ) -> dict[str, Any]:
        """获取URL内容。

        Args:
            url: 目标URL
            timeout: 超时时间（秒）
            wait_for_javascript: 是否等待JS执行
            wait_time: 等待时间（毫秒）
            screenshot: 是否截图

        Returns:
            获取结果字典
        """
        try:
            # 如果不需要等待JS，先用简单请求尝试
            if not wait_for_javascript and not screenshot:
                return await self._fetch_simple(url, timeout)

            # 需要动态渲染或截图，使用Playwright
            if not PLAYWRIGHT_AVAILABLE:
                self._logger.warning("Playwright not installed, falling back to simple fetch")
                return await self._fetch_simple(url, timeout)

            return await self._fetch_with_playwright(
                url,
                timeout,
                wait_for_javascript,
                wait_time,
                screenshot,
            )

        except Exception as e:
            self._logger.error("URL获取失败", url=url, error=str(e))
            raise

    async def _fetch_simple(self, url: str, timeout: int) -> dict[str, Any]:
        """简单HTTP请求获取。"""
        import httpx

        async with httpx.AsyncClient(timeout=timeout, follow_redirects=True) as client:
            response = await client.get(url)
            response.raise_for_status()

            html = response.text
            soup = BeautifulSoup(html, "html.parser")

            # 提取文本内容
            text = self._extract_text(soup)

            # 提取标题
            title = ""
            title_tag = soup.find("title")
            if title_tag:
                title = title_tag.get_text()

            # 提取链接
            links = self._extract_links(soup, url)

            return {
                "text": text,
                "html": html,
                "title": title,
                "links": links,
                "screenshot": None,
            }

    async def _fetch_with_playwright(
        self,
        url: str,
        timeout: int,
        wait_for_javascript: bool,
        wait_time: int,
        screenshot: bool,
    ) -> dict[str, Any]:
        """使用Playwright获取（支持动态渲染）。"""
        browser = await self._get_browser()

        async with browser.new_page() as page:
            # 设置超时
            page.set_default_timeout(timeout * 1000)

            # 导航到URL
            await page.goto(url, wait_until="domcontentloaded")

            # 等待JS执行
            if wait_for_javascript:
                await page.wait_for_timeout(wait_time)

            # 获取HTML
            html = await page.content()

            # 提取文本
            text = await page.evaluate(
                "() => { "
                "  // 移除script和style标签 "
                "  const clone = document.body.cloneNode(true); "
                "  clone.querySelectorAll('script, style').forEach(el => el.remove()); "
                "  return clone.innerText || clone.textContent; "
                "}"
            )

            # 获取标题
            title = await page.title()

            # 提取链接
            links = await page.evaluate(
                """() => {
                const links = [];
                document.querySelectorAll('a[href]').forEach(a => {
                    links.push(a.href);
                });
                return [...new Set(links)];
            }"""
            )

            # 截图
            screenshot_bytes = None
            if screenshot:
                screenshot_bytes = await page.screenshot(full_page=False)

            return {
                "text": text or "",
                "html": html,
                "title": title,
                "links": links,
                "screenshot": screenshot_bytes,
            }

    def _extract_text(self, soup: BeautifulSoup) -> str:
        """从BeautifulSoup对象提取纯文本。"""
        # 移除script和style标签
        for script in soup(["script", "style"]):
            script.decompose()

        # 获取文本
        text = soup.get_text()

        # 清理空白
        lines = (line.strip() for line in text.splitlines())
        chunks = (phrase.strip() for line in lines for phrase in line.split("  "))
        text = "\n".join(chunk for chunk in chunks if chunk)

        return text

    def _extract_links(self, soup: BeautifulSoup, base_url: str) -> list[str]:
        """提取所有链接。"""
        links = []
        for a in soup.find_all("a", href=True):
            href = a["href"]
            # 处理相对链接
            if href.startswith("/"):
                from urllib.parse import urlparse
                parsed = urlparse(base_url)
                href = f"{parsed.scheme}://{parsed.netloc}{href}"
            elif not href.startswith("http"):
                continue
            links.append(href)

        # 去重
        return list(dict.fromkeys(links))
