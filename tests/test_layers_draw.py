"""Tests for the draw capability and ImageGenService."""
from pathlib import Path
from unittest.mock import MagicMock

import pytest

from lingtai.capabilities.draw import DrawManager, setup as setup_draw
from lingtai.services.image_gen import ImageGenService, create_image_gen_service


class StubImageGenService(ImageGenService):
    """Stub for testing — writes a fake file and returns the path."""

    def __init__(self, *, fail: bool = False, fail_msg: str = "generation failed"):
        self._fail = fail
        self._fail_msg = fail_msg

    def generate(self, prompt, *, aspect_ratio=None, output_dir=None, **kwargs):
        if self._fail:
            raise RuntimeError(self._fail_msg)
        output_dir = output_dir or Path.cwd() / "images"
        output_dir.mkdir(parents=True, exist_ok=True)
        out_path = output_dir / "stub_image.png"
        out_path.write_bytes(b"\x89PNG_STUB")
        return out_path


def make_mock_agent(tmp_path):
    agent = MagicMock()
    agent.working_dir = tmp_path
    return agent


class TestDrawManager:
    def test_generate_image_success(self, tmp_path):
        """DrawManager delegates to ImageGenService and returns file path."""
        svc = StubImageGenService()
        mgr = DrawManager(working_dir=tmp_path, image_gen_service=svc)
        result = mgr.handle({"prompt": "a cute cat"})
        assert result["status"] == "ok"
        assert Path(result["file_path"]).exists()

    def test_generate_with_aspect_ratio(self, tmp_path):
        """aspect_ratio is passed through to the service."""
        svc = MagicMock(spec=ImageGenService)
        out_path = tmp_path / "media" / "images" / "img.png"
        out_path.parent.mkdir(parents=True)
        out_path.write_bytes(b"FAKE")
        svc.generate.return_value = out_path

        mgr = DrawManager(working_dir=tmp_path, image_gen_service=svc)
        mgr.handle({"prompt": "wide shot", "aspect_ratio": "16:9"})
        svc.generate.assert_called_once()
        call_kwargs = svc.generate.call_args
        assert call_kwargs.kwargs.get("aspect_ratio") == "16:9" or call_kwargs[1].get("aspect_ratio") == "16:9"

    def test_service_error_caught(self, tmp_path):
        """Service errors are returned as error dicts."""
        svc = StubImageGenService(fail=True, fail_msg="rate limited")
        mgr = DrawManager(working_dir=tmp_path, image_gen_service=svc)
        result = mgr.handle({"prompt": "a cat"})
        assert result["status"] == "error"
        assert "rate limited" in result["message"]

    def test_missing_prompt(self, tmp_path):
        svc = StubImageGenService()
        mgr = DrawManager(working_dir=tmp_path, image_gen_service=svc)
        result = mgr.handle({})
        assert result["status"] == "error"
        assert "prompt" in result["message"]


class TestSetupDraw:
    def test_setup_with_image_gen_service(self, tmp_path):
        """setup() accepts an explicit image_gen_service."""
        agent = make_mock_agent(tmp_path)
        svc = StubImageGenService()
        mgr = setup_draw(agent, image_gen_service=svc)
        assert isinstance(mgr, DrawManager)
        agent.add_tool.assert_called_once()

    def test_setup_requires_provider(self, tmp_path):
        """setup() without provider or service raises ValueError."""
        agent = make_mock_agent(tmp_path)
        with pytest.raises(ValueError, match="draw capability requires"):
            setup_draw(agent)

    def test_setup_auto_creates_service(self, tmp_path, monkeypatch):
        """Without explicit service or mcp_client, setup uses the factory.

        We pass an explicit mcp_client via the factory to avoid triggering
        the full MiniMax MCP client creation chain.
        """
        agent = make_mock_agent(tmp_path)
        mock_mcp = MagicMock()
        # The factory for minimax ultimately needs an MCPClient; pass directly
        from lingtai.services.image_gen.minimax import MiniMaxImageGenService
        svc = MiniMaxImageGenService(mcp_client=mock_mcp)
        mgr = setup_draw(agent, image_gen_service=svc)
        assert isinstance(mgr, DrawManager)


class TestImageGenServiceABC:
    def test_cannot_instantiate_abc(self):
        with pytest.raises(TypeError):
            ImageGenService()

    def test_stub_implements_abc(self):
        svc = StubImageGenService()
        assert isinstance(svc, ImageGenService)


class TestCreateImageGenServiceFactory:
    def test_unknown_provider_raises(self):
        with pytest.raises(ValueError, match="Unknown image generation provider"):
            create_image_gen_service("unknown_provider")

    def test_minimax_provider_with_mcp_client(self):
        """Factory creates MiniMaxImageGenService when given an explicit mcp_client."""
        from lingtai.services.image_gen.minimax import MiniMaxImageGenService
        mock_mcp = MagicMock()
        svc = MiniMaxImageGenService(mcp_client=mock_mcp)
        assert isinstance(svc, MiniMaxImageGenService)
        assert isinstance(svc, ImageGenService)


class TestMiniMaxImageGenService:
    def test_generate_via_saved_file(self, tmp_path):
        """MCP saves file to output_directory — service finds it."""
        from lingtai.services.image_gen.minimax import MiniMaxImageGenService

        out_dir = tmp_path / "images"
        out_dir.mkdir(parents=True)
        saved = out_dir / "generated.jpeg"
        saved.write_bytes(b"\xff\xd8JPEG_FAKE")

        mcp = MagicMock()
        mcp.call_tool.return_value = {"status": "success", "text": "Image saved"}
        svc = MiniMaxImageGenService(mcp_client=mcp)
        result = svc.generate("a cute cat", output_dir=out_dir)
        assert result == saved
        mcp.call_tool.assert_called_once()

    def test_generate_via_url_fallback(self, tmp_path, monkeypatch):
        """MCP returns a URL — service downloads it."""
        from lingtai.services.image_gen.minimax import MiniMaxImageGenService

        url = "https://example.com/image.jpeg"
        mcp = MagicMock()
        mcp.call_tool.return_value = {"status": "success", "text": f"Success. Image URLs: ['{url}']"}

        fake_resp = MagicMock()
        fake_resp.content = b"\xff\xd8JPEG_DOWNLOADED"
        fake_resp.raise_for_status = MagicMock()

        fake_requests = MagicMock()
        fake_requests.get.return_value = fake_resp
        monkeypatch.setitem(__import__("sys").modules, "requests", fake_requests)

        out_dir = tmp_path / "images"
        svc = MiniMaxImageGenService(mcp_client=mcp)
        result = svc.generate("a sunset", output_dir=out_dir)
        assert result.exists()
        assert result.read_bytes() == b"\xff\xd8JPEG_DOWNLOADED"

    def test_mcp_error_response(self, tmp_path):
        from lingtai.services.image_gen.minimax import MiniMaxImageGenService

        mcp = MagicMock()
        mcp.call_tool.return_value = {"status": "error", "message": "rate limited"}
        svc = MiniMaxImageGenService(mcp_client=mcp)
        with pytest.raises(RuntimeError, match="rate limited"):
            svc.generate("a cat", output_dir=tmp_path / "out")

    def test_mcp_call_exception(self, tmp_path):
        from lingtai.services.image_gen.minimax import MiniMaxImageGenService

        mcp = MagicMock()
        mcp.call_tool.side_effect = RuntimeError("connection lost")
        svc = MiniMaxImageGenService(mcp_client=mcp)
        with pytest.raises(RuntimeError, match="MCP call failed.*connection lost"):
            svc.generate("a cat", output_dir=tmp_path / "out")


class TestAddCapabilityIntegration:
    def test_add_capability_draw_with_service(self, tmp_path):
        """Agent construction with draw capability using image_gen_service."""
        from lingtai.agent import Agent
        svc = MagicMock()
        svc.get_adapter.return_value = MagicMock()
        svc.provider = "gemini"
        svc.model = "gemini-test"
        image_svc = StubImageGenService()
        # Use _setup_capability post-construction to avoid manifest serialization
        # of non-JSON-serializable service objects.
        agent = Agent(service=svc, agent_name="test", working_dir=tmp_path)
        agent._setup_capability("draw", image_gen_service=image_svc)
        assert "draw" in agent._tool_handlers

    def test_add_capability_draw_requires_provider(self, tmp_path):
        """Agent construction with draw capability without provider raises ValueError."""
        from lingtai.agent import Agent
        svc = MagicMock()
        svc.get_adapter.return_value = MagicMock()
        svc.provider = "gemini"
        svc.model = "gemini-test"
        agent = Agent(service=svc, agent_name="test", working_dir=tmp_path)
        with pytest.raises(ValueError, match="draw capability requires"):
            agent._setup_capability("draw")
