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
  - `generate_video`: Generates video from a text prompt. Supports up to 3 `reference_images` with custom types (`ASSET` or `STYLE`).
  - `generate_video_from_image`: Generates video from a starter image (PNG, JPEG, WebP) and a prompt. Treats image as the exact starting frame.
  - `extend_video`: Extends an existing video (by ID/URI or path) by 7 seconds with a prompt.
  - `generate_extended_sequence`: Generates a sequence of extended videos from multiple sequential prompts and automatically combines them using `ffmpeg` (if available on the system).
  - `list_generated_videos`: Lists all videos in the output directory.
  - `get_video_info`: Retrieves file parameters and metadata for a specific video.

## Reference Image Guide & Prompting
Veo 3.1 supports up to 3 reference images to guide generation. There are two major modes:

### 1. ASSET (Subject/Identity Reference)
- **Purpose**: Clones subject identity, faces, clothing, or specific objects.
- **Critical Prompt Rule**: The prompt must **explicitly describe** the subject/identity being referenced. 
- **Example Prompt**: "A cinematic video of the man from the reference image laughing, close up, high detail."
- **Failure Mode**: If the prompt does not describe the subject (e.g., "A beach scene"), Veo will completely ignore the reference image.

### 2. STYLE (Aesthetic/Vibe Reference)
- **Purpose**: Clones color palette, artistic medium, lighting, and general aesthetic.
- **Example Prompt**: "A futuristic city in the style of the reference image."

### Difference from Starter Image (Image-to-Video)
- **Starter Image** (`generate_video_from_image`): The image acts as the **exact first frame**. The video animates forward from there.
- **Reference Image** (`generate_video` with `reference_images`): The image acts as an **ingredient**. The video does NOT start with this image, but uses it to paint the subject or style dynamically. Recommended for preserving faces or character consistency.

## Video Extension (`extend_video` & `generate_extended_sequence`)
- **Action**: Extends previous video sequences by **7 seconds**.
- **Supported Models**: `veo-3.1-generate-preview` or `veo-3.1-fast-generate-preview` (Veo Lite or Veo 2 do NOT support extension).
- **Constraints**:
  - Input video length must be **≤ 141 seconds**.
  - Maximum final output length is **148 seconds**.
  - Initial video must be `720p` at either `16:9` or `9:16` aspect ratio.
- **Prompt Strategy**: Describe a **seamless continuation** of the action from the end of the previous video. Describe the next sequential events rather than restarting or re-introducing the scene.

### Sequential Generation and Combining (`generate_extended_sequence`)
- **What it does**: Takes either simple `prompts` (strings) or advanced `segments` (objects with individual prompts and reference images). Generates the base segment, then sequentially extends the previous segment's API URI for each subsequent segment.
- **Per-Segment Reference Images**: Reference images are only supported for the very first segment (the base generation, Segment 1). Subsequent segments (Segment 2+) are generated using video extension, which does not support reference images.
- **Example structure**:
  - `segments[0]`: prompt: `"A cat walking down the street."` (Can use reference images to define the cat's style/identity)
  - `segments[1]`: prompt: `"The cat starts chasing a red ball."` (Video extension: cannot use reference images, continue the action with text prompt)
  - `segments[2]`: prompt: `"A dog barks at the cat."` (Video extension: cannot use reference images)
- **Combining**: If `ffmpeg` is present on the host system, the tool uses the lossless `concat` demuxer to merge all segments into a single cohesive video file (`veo3_combined_<timestamp>.mp4`). If `ffmpeg` is missing, it skips merging but returns all individual files safely.

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
- Requests to make, generate, synthesize, extend, or animate videos or movies.
- Interacting with Google Veo 3 or Veo 2 models.
- Reviewing or retrieving local video generation assets.
