"""Python gRPC 服务实现。

专注于文档处理功能：
- 文档解析
- OCR 识别
- 文本分块
- URL 内容获取
- 评测服务

数据分析功能：
- 数理统计
- 趋势分析
- 数据洞察
"""

import asyncio
from concurrent import futures
from typing import TYPE_CHECKING

import grpc
from core import get_logger
from services.document import (
    parse_document,
    ocr_image,
    chunk_text,
    fetch_url,
    ChunkStrategy,
)

from proto import docreader_pb2
from proto import docreader_pb2_grpc
from proto import evaluation_pb2_grpc

if TYPE_CHECKING:
    from grpc import ServicerContext

# 延迟导入评测服务（避免启动时的依赖问题）
_evaluation_servicer = None


def get_evaluation_servicer():
    """获取评测服务实例。"""
    global _evaluation_servicer
    if _evaluation_servicer is None:
        from services.evaluation.service import EvaluationServicer
        _evaluation_servicer = EvaluationServicer()
    return _evaluation_servicer


class DocumentReaderServicer(docreader_pb2_grpc.DocumentReaderServiceServicer):
    """文档解析 gRPC 服务实现。

    实现文档解析、OCR识别、文本分块、URL获取等功能。
    使用同步方法包装异步服务函数 (Windows 兼容性)。
    """

    def __init__(self) -> None:
        self._logger = get_logger(__name__)

    def ParseDocument(self, request, context: "grpc.ServicerContext"):
        """解析文档。"""
        self._logger.info("收到 ParseDocument 请求")

        # 在事件循环中运行异步函数
        result = asyncio.run(self._parse_document_async(request))
        return result

    async def _parse_document_async(self, request) -> "docreader_pb2.ParseResponse":
        """异步解析文档。"""
        # 确定数据源
        source = None
        if request.file_path:
            source = request.file_path
        elif request.url:
            fetch_result = await fetch_url(request.url)
            if not fetch_result["success"]:
                return docreader_pb2.ParseResponse(
                    success=False,
                    error=f"URL获取失败: {fetch_result['error']}"
                )
            source = fetch_result["text"].encode("utf-8")
        elif request.content:
            source = request.content
        else:
            return docreader_pb2.ParseResponse(
                success=False,
                error="未指定数据源（file_path/url/content）"
            )

        # 解析选项
        options = request.options
        if options:
            include_metadata = options.include_metadata
            extract_tables = options.extract_tables
            extract_images = options.extract_images
        else:
            include_metadata = True
            extract_tables = False
            extract_images = False

        # 调用解析函数
        result = await parse_document(
            source=source,
            format=request.format or "",
            include_metadata=include_metadata,
            extract_tables=extract_tables,
            extract_images=extract_images,
        )

        # 构建响应
        response = docreader_pb2.ParseResponse(
            success=result["success"],
            text=result.get("text", ""),
            error=result.get("error", "")
        )

        # 添加元数据
        if result.get("metadata") and include_metadata:
            metadata = result["metadata"]
            doc_metadata = docreader_pb2.DocumentMetadata(
                format=metadata.get("format", ""),
                page_count=metadata.get("page_count", 0),
                char_count=metadata.get("char_count", 0)
            )
            for key, value in metadata.get("properties", {}).items():
                doc_metadata.properties[key] = str(value)
            response.metadata.CopyFrom(doc_metadata)

        # 添加表格
        for table in result.get("tables", []):
            table_data = docreader_pb2.TableData(
                page=table.get("page", 0),
                table_index=table.get("table_index", 0),
                markdown=table.get("markdown", "")
            )
            for row_data in table.get("rows", []):
                row = docreader_pb2.Row()
                row.cells.extend(row_data.get("cells", []))
                table_data.rows.append(row)
            response.tables.append(table_data)

        # 添加图片
        for img in result.get("images", []):
            img_info = docreader_pb2.ImageInfo(
                page=img.get("page", 0),
                image_index=img.get("image_index", 0),
                path=img.get("path", ""),
                width=img.get("width", 0),
                height=img.get("height", 0)
            )
            response.images.append(img_info)

        return response

    def ParseBatch(self, request, context: "grpc.ServicerContext"):
        """批量解析文档。"""
        self._logger.info("收到 ParseBatch 请求", count=len(request.requests))

        responses = []
        for req in request.requests:
            resp = self.ParseDocument(req, context)
            responses.append(resp)

        response = docreader_pb2.ParseBatchResponse()
        response.responses.extend(responses)
        return response

    def OCRImage(self, request, context: "grpc.ServicerContext"):
        """OCR识别图片。"""
        self._logger.info("收到 OCRImage 请求")
        result = asyncio.run(self._ocr_image_async(request))
        return result

    async def _ocr_image_async(self, request) -> "docreader_pb2.OCRResponse":
        """异步 OCR 识别。"""
        # 确定数据源
        source = None
        if request.file_path:
            source = request.file_path
        elif request.image_data:
            source = request.image_data
        elif request.url:
            import httpx
            async with httpx.AsyncClient() as client:
                response = await client.get(request.url)
                source = response.content
        else:
            return docreader_pb2.OCRResponse(
                success=False,
                text="",
                confidence=0.0,
                error="未指定数据源（file_path/image_data/url）"
            )

        # OCR选项
        options = request.options
        if options:
            det = options.det
            rec = options.rec
            use_cls = options.use_cls
            return_details = options.return_details
        else:
            det = True
            rec = True
            use_cls = False
            return_details = False

        result = await ocr_image(
            source=source,
            engine=request.engine or "paddleocr",
            language=request.language or "chi_sim",
            det=det,
            rec=rec,
            use_cls=use_cls,
            return_details=return_details,
        )

        # 构建响应
        response = docreader_pb2.OCRResponse(
            success=result["success"],
            text=result.get("text", ""),
            confidence=result.get("confidence", 0.0),
            error=result.get("error", "")
        )

        # 添加详细blocks
        for block in result.get("blocks", []):
            ocr_block = docreader_pb2.OCRBlock()
            if "box" in block:
                box_data = block["box"]
                rect = docreader_pb2.Rect(
                    x=box_data.get("x", 0),
                    y=box_data.get("y", 0),
                    width=box_data.get("width", 0),
                    height=box_data.get("height", 0)
                )
                ocr_block.box.CopyFrom(rect)
            ocr_block.text = block.get("text", "")
            ocr_block.confidence = block.get("confidence", 0.0)
            response.blocks.append(ocr_block)

        return response

    def OCRBatch(self, request, context: "grpc.ServicerContext"):
        """批量OCR识别。"""
        self._logger.info("收到 OCRBatch 请求", count=len(request.requests))

        responses = []
        for index, req in enumerate(request.requests):
            response = self.OCRImage(req, context)
            batch_resp = docreader_pb2.OCRBatchResponse(index=index, response=response)
            responses.append(batch_resp)

        return iter(responses)

    def ChunkDocument(self, request, context: "grpc.ServicerContext"):
        """文档分块。"""
        self._logger.info("收到 ChunkDocument 请求", text=request.text[:50] if request.text else "")
        try:
            result = asyncio.run(self._chunk_document_async(request))
            return result
        except Exception as e:
            self._logger.error("ChunkDocument 处理失败", error=str(e))
            raise

    async def _chunk_document_async(self, request) -> "docreader_pb2.ChunkResponse":
        """异步文档分块。"""
        # 映射策略枚举
        strategy_map = {
            docreader_pb2.ChunkStrategy.PARAGRAPH: ChunkStrategy.PARAGRAPH,
            docreader_pb2.ChunkStrategy.SENTENCE: ChunkStrategy.SENTENCE,
            docreader_pb2.ChunkStrategy.FIXED_SIZE: ChunkStrategy.FIXED_SIZE,
            docreader_pb2.ChunkStrategy.SEMANTIC: ChunkStrategy.SEMANTIC,
            docreader_pb2.ChunkStrategy.RECURSIVE: ChunkStrategy.RECURSIVE,
        }

        strategy = strategy_map.get(request.strategy, ChunkStrategy.PARAGRAPH)

        # 分块选项
        options = request.options
        if options:
            chunk_size = options.chunk_size
            chunk_overlap = options.chunk_overlap
            separator = options.separator
            min_chunk_size = options.min_chunk_size
        else:
            chunk_size = 1000
            chunk_overlap = 200
            separator = "\n\n"
            min_chunk_size = 100

        result = await chunk_text(
            text=request.text,
            strategy=strategy,
            chunk_size=chunk_size,
            chunk_overlap=chunk_overlap,
            separator=separator,
            min_chunk_size=min_chunk_size,
        )

        # 构建响应
        response = docreader_pb2.ChunkResponse(
            total_count=result.get("total_count", len(result.get("chunks", [])))
        )

        for chunk_data in result.get("chunks", []):
            chunk = docreader_pb2.Chunk(
                text=chunk_data.get("text", ""),
                index=chunk_data.get("index", 0),
                start_pos=chunk_data.get("start_pos", 0),
                end_pos=chunk_data.get("end_pos", 0)
            )
            for key, value in chunk_data.get("metadata", {}).items():
                chunk.metadata[key] = str(value)
            response.chunks.append(chunk)

        return response

    def FetchURL(self, request, context: "grpc.ServicerContext"):
        """获取URL内容。"""
        self._logger.info("收到 FetchURL 请求", url=request.url)
        result = asyncio.run(self._fetch_url_async(request))
        return result

    async def _fetch_url_async(self, request) -> "docreader_pb2.FetchURLResponse":
        """异步获取URL内容。"""
        options = request.options
        if options:
            # Use 30 as default if timeout is 0 (protobuf default)
            timeout = options.timeout or 30
            wait_for_javascript = options.wait_for_javascript
            wait_time = options.wait_time or 1000
            screenshot = options.screenshot
        else:
            timeout = 30
            wait_for_javascript = False
            wait_time = 1000
            screenshot = False

        result = await fetch_url(
            url=request.url,
            timeout=timeout,
            wait_for_javascript=wait_for_javascript,
            wait_time=wait_time,
            screenshot=screenshot,
        )

        # 构建响应
        response = docreader_pb2.FetchURLResponse(
            success=result["success"],
            text=result.get("text", ""),
            html=result.get("html", ""),
            title=result.get("title", ""),
            screenshot=result.get("screenshot", b""),
            error=result.get("error", "")
        )
        response.links.extend(result.get("links", []))

        return response


def create_grpc_server(port: int = 50051, include_analytics: bool = False):
    """创建 gRPC 服务器。

    Args:
        port: 监听端口
        include_analytics: 是否包含数据分析服务

    Returns:
        gRPC 服务器实例
    """
    from concurrent import futures

    # 使用同步服务器，使用默认选项
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))

    # 添加 DocumentReader 服务
    docreader_pb2_grpc.add_DocumentReaderServiceServicer_to_server(
        DocumentReaderServicer(), server
    )

    # 添加 Evaluation 服务
    try:
        evaluation_servicer = get_evaluation_servicer()
        evaluation_pb2_grpc.add_EvaluationServiceServicer_to_server(
            evaluation_servicer, server
        )
    except Exception as e:
        logger = get_logger(__name__)
        logger.warning(f"Failed to register EvaluationService", error=str(e))

    server.add_insecure_port(f'127.0.0.1:{port}')
    return server


async def serve_grpc(port: int = 50051, enable_analytics: bool = True) -> None:
    """启动 gRPC 服务器。

    Args:
        port: 监听端口
        enable_analytics: 是否启动数据分析服务
    """
    import threading

    logger = get_logger(__name__)

    # 创建同步服务器
    server = create_grpc_server(port)

    logger.info(f"启动 gRPC 服务器，端口: {port}")

    # 在单独线程中启动服务器
    def start_server():
        server.start()
        server.wait_for_termination()

    server_thread = threading.Thread(target=start_server, daemon=True)
    server_thread.start()

    logger.info("gRPC 服务器已启动")

    # 启动数据分析服务（如果启用）
    analytics_server = None
    analytics_thread = None
    if enable_analytics:
        try:
            from grpc_service.analytics_servicer import create_analytics_server
            analytics_port = 50053
            analytics_server = create_analytics_server(analytics_port)

            def start_analytics():
                analytics_server.start()
                analytics_server.wait_for_termination()

            analytics_thread = threading.Thread(target=start_analytics, daemon=True)
            analytics_thread.start()

            logger.info(f"数据分析 gRPC 服务器已启动，端口: {analytics_port}")
        except Exception as e:
            logger.warning(f"数据分析服务启动失败", error=str(e))

    # 保持主线程运行
    try:
        while server_thread.is_alive():
            await asyncio.sleep(1)
    except KeyboardInterrupt:
        logger.info("正在关闭 gRPC 服务器...")
        server.stop(0)
        if analytics_server:
            analytics_server.stop(0)
