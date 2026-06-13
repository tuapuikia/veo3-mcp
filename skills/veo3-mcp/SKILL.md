---
name: veo3-mcp
description: Gemini CLI extension for Google Veo 3 - generate, list, and inspect high-quality videos using text prompts or starting images.
---

# veo3-mcp Skill

This skill integrates the `veo3-mcp` Go MCP server tools to allow Gemini CLI to generate, inspect, and manage videos. It supports text-to-video generation, image-to-video generation, listing generated files, and retrieving metadata with secure boundary verification.

## Roles & Responsibilities
- **Video Generation**: Create high-quality 8-second 720p videos with audio from natural language text prompts.
- **Image-to-Video Animation**: Transform a starting image into a moving video sequence using dynamic motion descriptors.
- **Output Management**: List all generated videos and inspect their file parameters (size, path, timestamps) securely.

## Global Prompt Rules
- **Triggers**: Activated by prompts requesting video generation, text-to-video, image-to-video, movie/clip creation, or any mention of **veo** or video generation tools.
- **Default Outputs**: Save all videos directly to `./veo-output` (or the configured `VEO3_OUTPUT_DIR` environment directory).
- **Directory Traversal Protection**: Ensure file paths do not escape the designated output directory boundary.

## Tool Selection & Priority
- **MCP Server**: `veo3-mcp`
- **Core Tools**:
  - `generate_video`: Generates video from a text prompt.
  - `generate_video_from_image`: Generates video from a starter image (PNG, JPEG, WebP) and a prompt.
  - `list_generated_videos`: Lists all videos in the output directory.
  - `get_video_info`: Retrieves file parameters and metadata for a specific video.

## Configuration & Environment Fallbacks
- **API Authentication**:
  - `VEO_GEMINI_API_KEY` (Primary)
  - `VEO_GOOGLE_API_KEY` (Primary)
  - `GEMINI_API_KEY` (Fallback)
  - `GOOGLE_API_KEY` (Fallback)
  - `GEMINI_CLI_APP` (Fallback)
- **Model Selection**:
  - `VEO_DEFAULT_MODEL`: Overrides the default video model (defaults to `veo-3.1-fast-generate-preview`).
  - **Natural Language Parsing**: Allows specifying the model inline within the text prompt (e.g., "sunset with veo 3.1 lite" or "sunset model veo-custom-model"). The prompt is automatically cleaned up and the selected model is used.
  - **Supported Models**: `veo-3.1-fast-generate-preview` (default), `veo-3.1-generate-preview`, `veo-3.1-lite-generate-preview`, `veo-2.0-generate-001`. Arbitrary custom model overrides outside this list are also accepted.
- **Storage**:
  - `VEO3_OUTPUT_DIR`: Overrides the default `./veo-output` storage directory.

## When to use this skill
- Requests to make, generate, synthesize, or animate videos or movies.
- Interacting with Google Veo 3 or Veo 2 models.
- Reviewing or retrieving local video generation assets.
