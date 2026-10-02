"""Regression coverage for document/RPC boundary and resource lifecycle bugs."""

from types import SimpleNamespace
from unittest.mock import AsyncMock, Mock

import pytest
from core.file_access import validated_input_path
from grpc_service.servicer import DocumentReaderServicer
from proto import docreader_pb2
from services.document import chunk_text, fetch_url
from services.document.chunking.strategies import chunk_by_sentence, chunk_recursive
from services.document.fetcher import URLFetcher


@pytest.mark.parametrize("text", ["无标点的末句", "第一句。末句没有标点"])
def test_sentence_preserves_unterminated_tail(text):
    assert "".join(chunk["text"] for chunk in chunk_by_sentence(text)) == text


def test_recursive_preserves_plain_text_and_small_tail():
    text = "这是一段没有任何分隔符的中文内容"
    chunks = chunk_recursive(text, chunk_size=5, chunk_overlap=0, min_chunk_size=100)
    assert "".join(chunk["text"] for chunk in chunks) == text
    assert all(len(chunk["text"]) <= 5 for chunk in chunks)
    assert [chunk["index"] for chunk in chunks] == list(range(len(chunks)))


@pytest.mark.parametrize("size,overlap", [(0, 0), (-1, 0), (3, 3), (3, 4), (3, -1)])
async def test_invalid_chunk_sizes_return_error(size, overlap):
    result = await chunk_text(
        "abcdef", strategy="fixed_size", chunk_size=size, chunk_overlap=overlap
    )
    assert result["error"]


async def test_grpc_chunk_absent_options_uses_defaults():
    req = docreader_pb2.ChunkRequest(text="abcdef", strategy=docreader_pb2.FIXED_SIZE)
    response = await DocumentReaderServicer()._chunk_document_async(req)
    assert [chunk.text for chunk in response.chunks] == ["abcdef"]


async def test_grpc_tables_accept_parser_row_lists(monkeypatch):
    monkeypatch.setattr(
        "grpc_service.servicer.parse_document",
        AsyncMock(
            return_value={
                "success": True,
                "text": "table",
                "tables": [{"index": 2, "rows": [["a", "b"], ["1", "2"]]}],
            }
        ),
    )
    req = docreader_pb2.ParseRequest(content=b"table", format="txt")
    response = await DocumentReaderServicer()._parse_document_async(req)
    assert response.tables[0].table_index == 2
    assert list(response.tables[0].rows[1].cells) == ["1", "2"]


@pytest.mark.parametrize("failed", [False, True])
async def test_fetch_wrapper_always_closes(monkeypatch, failed):
    fetcher = SimpleNamespace(fetch=AsyncMock(return_value={"text": "ok"}), close=AsyncMock())
    if failed:
        fetcher.fetch.side_effect = RuntimeError("fetch failed")
    monkeypatch.setattr("services.document.fetcher.URLFetcher", lambda: fetcher)
    result = await fetch_url("https://example.com")
    assert result["success"] is not failed
    fetcher.close.assert_awaited_once()


@pytest.mark.parametrize("failed", [False, True])
async def test_browser_page_is_awaited_and_closed(monkeypatch, failed):
    from core.url_access import Download

    monkeypatch.setattr(
        "services.document.fetcher.download_url",
        AsyncMock(return_value=Download(b"html", "https://example.com/", "text/html", 200)),
    )
    page = SimpleNamespace(
        route=AsyncMock(),
        route_web_socket=AsyncMock(),
        set_default_timeout=Mock(),
        goto=AsyncMock(),
        content=AsyncMock(return_value="html"),
        evaluate=AsyncMock(side_effect=["text", []]),
        title=AsyncMock(return_value="title"),
        close=AsyncMock(),
    )
    context = SimpleNamespace(
        route=AsyncMock(),
        route_web_socket=AsyncMock(),
        add_init_script=AsyncMock(),
        new_page=AsyncMock(return_value=page),
        close=AsyncMock(),
    )
    if failed:
        page.goto.side_effect = RuntimeError("navigation failed")
    fetcher = URLFetcher()
    monkeypatch.setattr(
        fetcher,
        "_get_browser",
        AsyncMock(return_value=SimpleNamespace(new_context=AsyncMock(return_value=context))),
    )
    if failed:
        with pytest.raises(RuntimeError):
            await fetcher._fetch_with_playwright("https://example.com", 1, False, 0, False)
    else:
        result = await fetcher._fetch_with_playwright("https://example.com", 1, False, 0, False)
        assert result["text"] == "text"
        assert "//" not in page.evaluate.call_args_list[0].args[0]
    context.close.assert_awaited_once()
    context.route.assert_awaited_once()
    page.route.assert_not_awaited()  # Context covers popup first navigation too.
    from core.url_access import Download

    monkeypatch.setattr(
        "services.document.fetcher.download_url",
        AsyncMock(return_value=Download(b"html", "https://example.com/docs/", "text/html", 200)),
    )
    route = SimpleNamespace(
        request=SimpleNamespace(method="GET", url="https://example.com/docs"),
        fulfill=AsyncMock(),
        abort=AsyncMock(),
    )
    handler = context.route.call_args.args[1]
    await handler(route)
    route.abort.assert_awaited_once()
    route.fulfill.assert_not_awaited()


def test_remote_file_boundary_blocks_traversal_and_symlinks(tmp_path, monkeypatch):
    allowed = tmp_path / "allowed"
    allowed.mkdir()
    inside = allowed / "valid.txt"
    inside.write_text("ok")
    outside = tmp_path / "outside.txt"
    outside.write_text("private")
    (allowed / "link.txt").symlink_to(outside)
    monkeypatch.setattr(
        "core.file_access.get_settings", lambda: SimpleNamespace(allowed_paths=[str(allowed)])
    )
    assert validated_input_path(str(inside)) == str(inside)
    for path in [outside, allowed / ".." / "outside.txt", allowed / "link.txt"]:
        with pytest.raises(ValueError, match="outside"):
            validated_input_path(str(path))


async def test_grpc_cannot_read_outside_allowed_paths(tmp_path, monkeypatch):
    path = tmp_path / "private.txt"
    path.write_text("private")
    monkeypatch.setattr("core.file_access.get_settings", lambda: SimpleNamespace(allowed_paths=[]))
    result = await DocumentReaderServicer()._parse_document_async(
        docreader_pb2.ParseRequest(file_path=str(path))
    )
    assert not result.success
    assert not result.text


async def test_grpc_url_parser_receives_original_bytes(monkeypatch):
    from core.url_access import Download

    parser = AsyncMock(return_value={"success": True, "text": "pdf text"})
    monkeypatch.setattr(
        "grpc_service.servicer.download_url",
        AsyncMock(
            return_value=Download(
                b"%PDF-content", "https://example.com/report.pdf", "application/pdf", 200
            )
        ),
    )
    monkeypatch.setattr("grpc_service.servicer.parse_document", parser)
    result = await DocumentReaderServicer()._parse_document_async(
        docreader_pb2.ParseRequest(url="https://example.com/report.pdf")
    )
    assert result.success
    assert parser.call_args.kwargs["source"] == b"%PDF-content"
    assert parser.call_args.kwargs["format"] == "pdf"
