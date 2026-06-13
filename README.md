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
Generate a video using Google Veo from a text prompt. Supports advanced configuration parameters and up to 3 reference images.
- **Arguments**:
  - `prompt` (string, required): Descriptive prompt.
  - `model` (string, optional): Veo model to use (e.g. `veo-3.1-fast-generate-preview`, `veo-3.1-generate-preview`).
  - `aspect_ratio` (string, optional): Aspect ratio (e.g., `"16:9"`, `"9:16"`, `"21:9"`, `"1:1"`, `"4:3"`).
  - `resolution` (string, optional): Resolution (`"720p"`, `"1080p"`).
  - `duration_seconds` (integer, optional): Duration (e.g., `5`, `6`, `7`, `8`).
  - `fps` (integer, optional): Frames per second (`24`, `30`).
  - `seed` (integer, optional): Random seed.
  - `generate_audio` (boolean, optional): Set to `true` to generate matching audio.
  - `negative_prompt` (string, optional): Elements to avoid in the video.
  - `person_generation` (string, optional): Controls person generation quality/behavior (e.g., `"dont_allow"`, `"allow_adult"`).
  - `reference_images` (array, optional): Up to 3 reference images to guide generation. Each item contains:
    - `path` (string, required): Absolute local path to image file.
    - `type` (string, required): `"ASSET"` (replicates subject identity/face) or `"STYLE"` (replicates aesthetic/theme).

#### Inline Prompt Model Selection (Natural Language)
Instead of passing the `model` parameter explicitly, you can specify it directly inside your prompt using natural language keywords or explicit instructions.
- **Examples**:
  - *"A soaring eagle over grand canyon **with veo 3.1 lite***"
  - *"Cat chasing a laser **using veo 2***"

### 2. `generate_video_from_image` (Image-to-Video / Starter Frame)
Animate a starting image with natural motion.
- **Arguments**:
  - `prompt` (string, required): Descriptive movement prompt.
  - `image_path` (string, required): Absolute local path to starter image file.
  - `model` (string, optional): Veo model to use.

> [!NOTE]
> **Starter Image vs Reference Image**:
> - **Starter Image** (`generate_video_from_image`): The image becomes the **exact starting frame** of the video, which then animates forward.
>   - *Prompting Guideline*: Say `"starting with image"` or `"animate this starting frame"`.
>   - *Example*: `"A puppy runs across the lawn starting with image /path/to/pup.jpg"`
> - **Reference Image** (`generate_video` with `reference_images`): The image acts as an **influence/ingredient** (replicating character face, identity, or aesthetic style) but is NOT the starting frame.
>   - *Prompting Guideline*: Say `"using reference image"` or `"in the style of reference image"`.
>   - *Example*: `"A cinematic video of the puppy sleeping on a rug using reference image /path/to/pup.jpg"` (The prompt **must explicitly describe** the subject, e.g. "puppy", so Veo knows to apply the reference face/identity).

### 3. `extend_video`
Extend an existing Veo-generated video by **7 seconds** per extension.
- **Arguments**:
  - `prompt` (string, required): Descriptive prompt describing the **continuation** of the scene.
  - `video_uri` (string, optional): Direct API URI of the source video (e.g., `input_file_0`).
  - `video_path` (string, optional): Absolute path to the local video file.
  - `model` (string, optional): Model to use (must be Veo 3.1 or 3.1 Fast; Lite is not supported for extension).

#### Extension Guidelines & Max Length Limits
- **Max Input Length**: The input video being extended must be **≤ 141 seconds**.
- **Max Output Length**: Since each extension adds exactly 7 seconds, the theoretical maximum final video length is **148 seconds**.
- **Resolution & Aspect Ratio**: Initial video must be `720p` and use either `16:9` or `9:16` aspect ratio.
- **Prompt Strategy**: The extension prompt should specify **how the action progresses** seamlessly from the end of the previous clip. Avoid restarting the scene description.
- **Example Usage**:
  - *Original Prompt*: `"A cat pounces on a toy mouse on a living room rug."` (8 seconds)
  - *Extension Prompt*: `"The cat successfully grabs the toy in its paws and begins grooming it."` (Extends to 15 seconds)

### 4. `generate_extended_sequence`
Generate a sequence of extended videos from multiple sequential prompts and automatically combine them using `ffmpeg` (if available on the system).
- **Arguments**:
  - `prompts` (array of strings, optional): Simple mode. List of sequential prompt strings. (All segments will share global `reference_images` fallback).
  - `segments` (array of objects, optional): Advanced mode. List of segment objects. Each segment contains:
    - `prompt` (string, required): Prompt for this specific segment.
    - `reference_images` (array, optional): Up to 3 reference images (`path` and `type`) for this segment specifically.
  - `reference_images` (array, optional): Global reference images used as fallback for segments that do not define their own.
  - `model` (string, optional): Veo model to use (Veo 3.1 or Veo 3.1 Fast only).
  - `aspect_ratio` (string, optional): Aspect ratio (e.g. `"16:9"`).
  - `resolution` (string, optional): Resolution (e.g. `"720p"`).
  - `generate_audio` (boolean, optional): Generate matching audio.
- **How it works**:
  1. Resolves prompts or segments uniformly.
  2. Generates the base video segment using the first prompt (with its specified reference images).
  3. Sequentially extends using the previous segment's API URI for each subsequent prompt (using that segment's specific reference images).
  4. If `ffmpeg` is installed on the host system, it will combine all generated parts losslessly into a single `veo3_combined_<timestamp>.mp4` file. If `ffmpeg` is missing, it skips combination but returns all individual segments successfully.

### 5. `list_generated_videos`
List all generated videos in the output directory.

### 5. `get_video_info`
Get detailed information about a generated video file.
- **Arguments**:
  - `video_path` (string, required): Absolute or relative video file path.
