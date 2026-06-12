# Veo 3 MCP Server (Golang)

A high-performance Model Context Protocol (MCP) server written in Golang, providing tools for Google Veo 3 video generation. This is a direct, robust port of the Python `mcp-veo3` server.

## Features

- **Text-to-Video**: Generate stunning 8-second 720p videos with audio using Gemini/Veo 3.
- **Image-to-Video**: Animate starting images with natural motion using raw inline bytes (no complex pre-uploading required).
- **Environment Autodetect**: Supports multiple fallback configurations including `GEMINI_CLI_APP`.
- **Path Traversal Security**: Built-in boundary protection to prevent directory traversal escapes.

## Installation

Ensure you have Go 1.25+ installed.

```bash
cd veo3-mcp
go build -o veo3-mcp
```

## Running the Server

Start the MCP server with the desired transport type:

```bash
# Stdio transport (Default for MCP hosts like Claude Desktop)
./veo3-mcp --transport stdio --output-dir ~/Videos/Generated

# SSE transport
./veo3-mcp --transport sse --host 127.0.0.1 --port 8080
```

### Command Line Flags

- `--transport`: Transport type (`stdio` or `sse`). Default: `stdio`
- `--host`: Host address for SSE. Default: `127.0.0.1`
- `--port`: Port for SSE. Default: `8080`
- `--output-dir`: Absolute directory path to save generated videos. If empty, falls back to `VEO3_OUTPUT_DIR` environment or `./veo-output`.
- `--api-key`: Explicit Gemini API Key (overrides env vars).

## Environment Configuration

The server supports the following environment variable configurations:

### Authentication Fallbacks
1. `VEO_GEMINI_API_KEY` (Primary)
2. `VEO_GOOGLE_API_KEY` (Primary)
3. `GEMINI_API_KEY` (Fallback)
4. `GOOGLE_API_KEY` (Fallback)
5. `GEMINI_CLI_APP` (Fallback used by `gemini-cli`)

### Global Defaults
- `VEO_DEFAULT_MODEL`: Overrides the default video model (falls back to `veo-3.1-fast-generate-preview` if unset).
- `VEO3_OUTPUT_DIR`: Overrides the default video storage output directory (falls back to `./veo-output` if unset).

## MCP Tools

### 1. `generate_video`
Generate a video using Google Veo 3 from a text prompt.
- **Arguments**:
  - `prompt` (string, required): Descriptive prompt.
  - `model` (string, optional): Veo model to use (defaults to `veo-3.1-fast-generate-preview`; other valid models: `veo-3.1-generate-preview`, `veo-3.1-lite-generate-preview`, `veo-2.0-generate-001`; arbitrary custom models are also accepted).

### Inline Prompt Model Selection (Natural Language)
Instead of passing the `model` parameter explicitly, you can specify it directly inside your prompt using natural language keywords or explicit instructions. The server automatically detects the model, overrides the selection, and cleans the prompt before submission.
- **Examples**:
  - *"A futuristic city in the style of cyberpunk **using model veo-3.1-lite-generate-preview***"
  - *"A soaring eagle over grand canyon **with veo 3.1 lite***"
  - *"Cat chasing a laser **using veo 2***"
  - *"Majestic waterfall in forest **model veo-custom-model***" (Arbitrary custom models can also be overridden this way)

### 2. `generate_video_from_image`
Animate a starting image with natural motion.
- **Arguments**:
  - `prompt` (string, required): Descriptive movement prompt (also supports inline prompt model selection).
  - `image_path` (string, required): Absolute path or path relative to the output directory.
  - `model` (string, optional): Same model selection options as above.

### 3. `list_generated_videos`
List all generated videos in the output directory.

### 4. `get_video_info`
Get detailed information about a generated video file.
- **Arguments**:
  - `video_path` (string, required): Absolute or relative video file path.
