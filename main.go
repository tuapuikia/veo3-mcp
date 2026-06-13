package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/genai"
)

// Response structures matching Python server exactly
type VideoGenerationResponse struct {
	VideoPath      string   `json:"video_path"`
	Filename       string   `json:"filename"`
	Model          string   `json:"model"`
	Prompt         string   `json:"prompt"`
	NegativePrompt *string  `json:"negative_prompt"`
	GenerationTime float64  `json:"generation_time"`
	FileSize       int64    `json:"file_size"`
	AspectRatio    string   `json:"aspect_ratio"`
	VideoURI       string   `json:"video_uri,omitempty"`
}

type VideoFileInfo struct {
	Filename string  `json:"filename"`
	Path     string  `json:"path"`
	Size     int64   `json:"size"`
	SizeMB   float64 `json:"size_mb"`
	Created  string  `json:"created"`
	Modified string  `json:"modified"`
}

type VideoListResponse struct {
	Videos     []VideoFileInfo `json:"videos"`
	TotalCount int             `json:"total_count"`
	OutputDir  string          `json:"output_dir"`
}

type VideoInfoResponse struct {
	Filename string `json:"filename"`
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Created  string `json:"created"`
	Modified string `json:"modified"`
}

// ReferenceImageArgs matches input parameters for reference images
type ReferenceImageArgs struct {
	ImagePath     string `json:"image_path"`
	PathAlias     string `json:"path,omitempty"`
	ReferenceType string `json:"reference_type,omitempty"`
	TypeAlias     string `json:"type,omitempty"`
}

// Args structures for tool inputs
type GenerateVideoArgs struct {
	Prompt           string               `json:"prompt"`
	Model            string               `json:"model,omitempty"`
	AspectRatio      string               `json:"aspect_ratio,omitempty"`
	Resolution       string               `json:"resolution,omitempty"`
	DurationSeconds  *int32               `json:"duration_seconds,omitempty"`
	FPS              *int32               `json:"fps,omitempty"`
	NegativePrompt   string               `json:"negative_prompt,omitempty"`
	EnhancePrompt    *bool                `json:"enhance_prompt,omitempty"`
	GenerateAudio    *bool                `json:"generate_audio,omitempty"`
	Seed             *int32               `json:"seed,omitempty"`
	PersonGeneration string               `json:"person_generation,omitempty"`
	ReferenceImages  []ReferenceImageArgs `json:"reference_images,omitempty"`
}

type GenerateVideoFromImageArgs struct {
	Prompt    string `json:"prompt"`
	ImagePath string `json:"image_path"`
	Model     string `json:"model,omitempty"`
}

type ExtendVideoArgs struct {
	Prompt           string               `json:"prompt"`
	VideoURI         string               `json:"video_uri,omitempty"`
	VideoPath        string               `json:"video_path,omitempty"`
	Model            string               `json:"model,omitempty"`
	AspectRatio      string               `json:"aspect_ratio,omitempty"`
	Resolution       string               `json:"resolution,omitempty"`
	DurationSeconds  *int32               `json:"duration_seconds,omitempty"`
	FPS              *int32               `json:"fps,omitempty"`
	NegativePrompt   string               `json:"negative_prompt,omitempty"`
	EnhancePrompt    *bool                `json:"enhance_prompt,omitempty"`
	GenerateAudio    *bool                `json:"generate_audio,omitempty"`
	Seed             *int32               `json:"seed,omitempty"`
	PersonGeneration string               `json:"person_generation,omitempty"`
	ReferenceImages  []ReferenceImageArgs `json:"reference_images,omitempty"`
}

type SegmentArgs struct {
	Prompt          string               `json:"prompt"`
	ReferenceImages []ReferenceImageArgs `json:"reference_images,omitempty"`
}

type GenerateExtendedSequenceArgs struct {
	Prompts          []string             `json:"prompts,omitempty"`
	Segments         []SegmentArgs        `json:"segments,omitempty"`
	Model            string               `json:"model,omitempty"`
	AspectRatio      string               `json:"aspect_ratio,omitempty"`
	Resolution       string               `json:"resolution,omitempty"`
	DurationSeconds  *int32               `json:"duration_seconds,omitempty"`
	FPS              *int32               `json:"fps,omitempty"`
	NegativePrompt   string               `json:"negative_prompt,omitempty"`
	EnhancePrompt    *bool                `json:"enhance_prompt,omitempty"`
	GenerateAudio    *bool                `json:"generate_audio,omitempty"`
	Seed             *int32               `json:"seed,omitempty"`
	PersonGeneration string               `json:"person_generation,omitempty"`
	ReferenceImages  []ReferenceImageArgs `json:"reference_images,omitempty"`
}

type ExtendedSequenceResponse struct {
	CombinedVideoPath string                     `json:"combined_video_path,omitempty"`
	Segments          []VideoGenerationResponse `json:"segments"`
	CombinedSuccess   bool                       `json:"combined_success"`
	CombinedMessage   string                     `json:"combined_message"`
}

type GetVideoInfoArgs struct {
	VideoPath string `json:"video_path"`
}

var outputDir string

func main() {
	transportType := flag.String("transport", "stdio", "Transport type (stdio or sse)")
	host := flag.String("host", "127.0.0.1", "Host for SSE transport")
	port := flag.Int("port", 8080, "Port for SSE transport")
	cliOutputDir := flag.String("output-dir", "", "Directory to save generated videos")
	cliAPIKey := flag.String("api-key", "", "Gemini API key")
	flag.Parse()

	// 1. Resolve output directory
	outputDir = *cliOutputDir
	if outputDir == "" {
		outputDir = os.Getenv("VEO3_OUTPUT_DIR")
	}
	if outputDir == "" {
		outputDir = "./veo-output"
	}
	outputDir = filepath.Clean(outputDir)

	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create output directory %s: %v\n", outputDir, err)
		os.Exit(1)
	}

	// 2. Validate Authentication
	apiKey, err := ValidateAuthentication(*cliAPIKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// 3. Initialize Gemini Client
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize Gemini Client: %v\n", err)
		os.Exit(1)
	}

	// 4. Initialize MCP Server
	s := mcp.NewServer(&mcp.Implementation{
		Name:    "MCP Veo 3 Video Generator",
		Version: "1.1.0",
	}, nil)

	// --- generate_video ---
	mcp.AddTool(s, &mcp.Tool{
		Name:        "generate_video",
		Description: "Generate a video using Google Veo 3 from a text prompt. Veo 3 generates 8-second 720p videos with audio.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompt": map[string]any{
					"type":        "string",
					"description": "Text prompt describing the video to generate",
				},
				"model": map[string]any{
					"type":        "string",
					"description": "Veo model to use (defaults to veo-3.1-fast-generate-preview; other valid models: veo-3.1-generate-preview, veo-3.1-lite-generate-preview, veo-2.0-generate-001; arbitrary custom models are also accepted)",
					"default":     "veo-3.1-fast-generate-preview",
				},
				"aspect_ratio": map[string]any{
					"type":        "string",
					"description": "Aspect ratio for the generated video: '16:9' (landscape) or '9:16' (portrait)",
					"enum":        []string{"16:9", "9:16"},
				},
				"resolution": map[string]any{
					"type":        "string",
					"description": "Resolution: '720p' or '1080p'",
					"enum":        []string{"720p", "1080p"},
				},
				"duration_seconds": map[string]any{
					"type":        "integer",
					"description": "Duration of the clip in seconds",
				},
				"fps": map[string]any{
					"type":        "integer",
					"description": "Frames per second",
				},
				"negative_prompt": map[string]any{
					"type":        "string",
					"description": "Explicitly state what should not be included in the video",
				},
				"enhance_prompt": map[string]any{
					"type":        "boolean",
					"description": "Whether to use prompt rewriting logic to enhance quality",
				},
				"generate_audio": map[string]any{
					"type":        "boolean",
					"description": "Whether to generate audio along with the video",
				},
				"seed": map[string]any{
					"type":        "integer",
					"description": "RNG seed for consistent results",
				},
				"person_generation": map[string]any{
					"type":        "string",
					"description": "Person generation policy: 'dont_allow' or 'allow_adult'",
					"enum":        []string{"dont_allow", "allow_adult"},
				},
				"reference_images": map[string]any{
					"type":        "array",
					"description": "Up to 3 reference images to preserve subject/style appearance (Veo 3.1 only)",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"image_path": map[string]any{
								"type":        "string",
								"description": "Path to the local reference image file",
							},
							"reference_type": map[string]any{
								"type":        "string",
								"description": "Reference type: 'ASSET' (guides subject/person details) or 'STYLE' (guides theme/colors)",
								"enum":        []string{"ASSET", "STYLE"},
							},
						},
						"required": []string{"image_path"},
					},
				},
			},
			"required": []string{"prompt"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args json.RawMessage) (*mcp.CallToolResult, any, error) {
		var toolArgs GenerateVideoArgs
		if err := json.Unmarshal(args, &toolArgs); err != nil {
			return nil, nil, err
		}

		if strings.TrimSpace(toolArgs.Prompt) == "" {
			return formatJSONResult(nil, fmt.Errorf("prompt cannot be empty"))
		}

		model := toolArgs.Model
		prompt := toolArgs.Prompt

		parsedPrompt, detectedModel := parseModelAndPrompt(prompt)
		if detectedModel != "" {
			if model == "" {
				model = detectedModel
			}
			prompt = parsedPrompt
		}

		if model == "" {
			model = getDefaultModel()
		}

		// Prepare Config
		config := &genai.GenerateVideosConfig{
			AspectRatio:      toolArgs.AspectRatio,
			Resolution:       toolArgs.Resolution,
			DurationSeconds:  toolArgs.DurationSeconds,
			FPS:              toolArgs.FPS,
			NegativePrompt:   toolArgs.NegativePrompt,
			GenerateAudio:    toolArgs.GenerateAudio,
			Seed:             toolArgs.Seed,
			PersonGeneration: toolArgs.PersonGeneration,
		}
		if toolArgs.EnhancePrompt != nil {
			config.EnhancePrompt = *toolArgs.EnhancePrompt
		}

		// Resolve Reference Images
		refImages, err := resolveReferenceImages(toolArgs.ReferenceImages)
		if err != nil {
			return formatJSONResult(nil, err)
		}

		source := &genai.GenerateVideosSource{
			Prompt: prompt,
		}

		res, err := generateVideoHelper(ctx, client, model, source, config, refImages)
		return formatJSONResult(res, err)
	})

	// --- generate_video_from_image ---
	mcp.AddTool(s, &mcp.Tool{
		Name:        "generate_video_from_image",
		Description: "Generate a video using Google Veo 3 from an image and text prompt. Veo 3 generates 8-second 720p videos with audio.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompt": map[string]any{
					"type":        "string",
					"description": "Text prompt describing the video motion/action",
				},
				"image_path": map[string]any{
					"type":        "string",
					"description": "Path to the starting image file (can be absolute or relative to output directory)",
				},
				"model": map[string]any{
					"type":        "string",
					"description": "Veo model to use (defaults to veo-3.1-fast-generate-preview; other valid models: veo-3.1-generate-preview, veo-3.1-lite-generate-preview, veo-2.0-generate-001; arbitrary custom models are also accepted)",
					"default":     "veo-3.1-fast-generate-preview",
				},
			},
			"required": []string{"prompt", "image_path"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args json.RawMessage) (*mcp.CallToolResult, any, error) {
		var toolArgs GenerateVideoFromImageArgs
		if err := json.Unmarshal(args, &toolArgs); err != nil {
			return nil, nil, err
		}

		if strings.TrimSpace(toolArgs.Prompt) == "" {
			return formatJSONResult(nil, fmt.Errorf("prompt cannot be empty"))
		}

		imagePath := strings.TrimSpace(toolArgs.ImagePath)
		if imagePath == "" {
			return formatJSONResult(nil, fmt.Errorf("image_path cannot be empty"))
		}

		resolvedImagePath, err := resolveSafePath(imagePath)
		if err != nil {
			return formatJSONResult(nil, err)
		}

		if _, err := os.Stat(resolvedImagePath); os.IsNotExist(err) {
			return formatJSONResult(nil, fmt.Errorf("image file not found: %s", resolvedImagePath))
		}

		model := toolArgs.Model
		prompt := toolArgs.Prompt

		parsedPrompt, detectedModel := parseModelAndPrompt(prompt)
		if detectedModel != "" {
			if model == "" {
				model = detectedModel
			}
			prompt = parsedPrompt
		}

		if model == "" {
			model = getDefaultModel()
		}

		// Read image bytes
		imageBytes, err := os.ReadFile(resolvedImagePath)
		if err != nil {
			return formatJSONResult(nil, fmt.Errorf("failed to read image file: %w", err))
		}

		mimeType := "image/jpeg"
		if strings.HasSuffix(strings.ToLower(resolvedImagePath), ".png") {
			mimeType = "image/png"
		} else if strings.HasSuffix(strings.ToLower(resolvedImagePath), ".webp") {
			mimeType = "image/webp"
		}

		img := &genai.Image{
			ImageBytes: imageBytes,
			MIMEType:   mimeType,
		}

		source := &genai.GenerateVideosSource{
			Prompt: prompt,
			Image:  img,
		}

		res, err := generateVideoHelper(ctx, client, model, source, nil, nil)
		return formatJSONResult(res, err)
	})

	// --- extend_video ---
	mcp.AddTool(s, &mcp.Tool{
		Name:        "extend_video",
		Description: "Extend a previously generated Veo video by 7 seconds. Veo 3.1 & 3.1 Fast only. Input video must be 141s or less.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompt": map[string]any{
					"type":        "string",
					"description": "Text prompt describing the extension action/motion",
				},
				"video_uri": map[string]any{
					"type":        "string",
					"description": "Gemini API URI of previously generated video to extend (preferred, starts with https:// or gs://)",
				},
				"video_path": map[string]any{
					"type":        "string",
					"description": "Path to local video file of a previously generated video to extend",
				},
				"model": map[string]any{
					"type":        "string",
					"description": "Veo model to use (Veo 3.1 or Veo 3.1 Fast only; Lite is not supported for extension)",
					"default":     "veo-3.1-fast-generate-preview",
				},
				"aspect_ratio": map[string]any{
					"type":        "string",
					"description": "Aspect ratio (must match original video)",
					"enum":        []string{"16:9", "9:16"},
				},
				"resolution": map[string]any{
					"type":        "string",
					"description": "Resolution: '720p' or '1080p'",
					"enum":        []string{"720p", "1080p"},
				},
				"duration_seconds": map[string]any{
					"type":        "integer",
					"description": "Duration of the extension in seconds",
				},
				"fps": map[string]any{
					"type":        "integer",
					"description": "Frames per second",
				},
				"negative_prompt": map[string]any{
					"type":        "string",
					"description": "Explicitly state what should not be included in the extension",
				},
				"enhance_prompt": map[string]any{
					"type":        "boolean",
					"description": "Whether to use prompt rewriting logic",
				},
				"generate_audio": map[string]any{
					"type":        "boolean",
					"description": "Whether to generate audio along with the video",
				},
				"seed": map[string]any{
					"type":        "integer",
					"description": "RNG seed",
				},
				"person_generation": map[string]any{
					"type":        "string",
					"description": "Person generation policy: 'dont_allow' or 'allow_adult'",
					"enum":        []string{"dont_allow", "allow_adult"},
				},
			},
			"required": []string{"prompt"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args json.RawMessage) (*mcp.CallToolResult, any, error) {
		var toolArgs ExtendVideoArgs
		if err := json.Unmarshal(args, &toolArgs); err != nil {
			return nil, nil, err
		}

		if strings.TrimSpace(toolArgs.Prompt) == "" {
			return formatJSONResult(nil, fmt.Errorf("prompt cannot be empty"))
		}

		if toolArgs.VideoURI == "" && toolArgs.VideoPath == "" {
			return formatJSONResult(nil, fmt.Errorf("either video_uri or video_path must be provided to extend a video"))
		}

		model := toolArgs.Model
		prompt := toolArgs.Prompt

		parsedPrompt, detectedModel := parseModelAndPrompt(prompt)
		if detectedModel != "" {
			if model == "" {
				model = detectedModel
			}
			prompt = parsedPrompt
		}

		if model == "" {
			model = getDefaultModel()
		}

		// Reference images are not supported for video extensions in Google GenAI API
		var refImages []*genai.VideoGenerationReferenceImage
		if len(toolArgs.ReferenceImages) > 0 {
			fmt.Fprintf(os.Stderr, "Warning: Reference images are not supported for video extensions. Skipping reference images.\n")
		}

		// Resolve the video source
		video, err := resolveVideo(toolArgs.VideoURI, toolArgs.VideoPath)
		if err != nil {
			return formatJSONResult(nil, err)
		}

		config := &genai.GenerateVideosConfig{
			AspectRatio:      toolArgs.AspectRatio,
			Resolution:       toolArgs.Resolution,
			DurationSeconds:  toolArgs.DurationSeconds,
			FPS:              toolArgs.FPS,
			NegativePrompt:   toolArgs.NegativePrompt,
			GenerateAudio:    toolArgs.GenerateAudio,
			Seed:             toolArgs.Seed,
			PersonGeneration: toolArgs.PersonGeneration,
		}
		if toolArgs.EnhancePrompt != nil {
			config.EnhancePrompt = *toolArgs.EnhancePrompt
		}

		source := &genai.GenerateVideosSource{
			Prompt: prompt,
			Video:  video,
		}

		res, err := generateVideoHelper(ctx, client, model, source, config, refImages)
		return formatJSONResult(res, err)
	})

	// --- generate_extended_sequence ---
	mcp.AddTool(s, &mcp.Tool{
		Name:        "generate_extended_sequence",
		Description: "Generate a sequence of extended videos from multiple sequential prompts and automatically combine them using ffmpeg if available. Supports per-segment prompts and reference images.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompts": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "string",
					},
					"description": "List of sequential prompt strings. Simple mode (all segments will share global reference_images if provided).",
				},
				"segments": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"prompt": map[string]any{
								"type":        "string",
								"description": "Prompt for this segment",
							},
							"reference_images": map[string]any{
								"type": "array",
								"items": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"path": map[string]any{
											"type":        "string",
											"description": "Absolute local path to image",
										},
										"type": map[string]any{
											"type":        "string",
											"description": "Reference type: 'ASSET' or 'STYLE'",
										},
									},
									"required": []string{"path"},
								},
								"description": "Optional list of up to 3 reference images for this specific segment",
							},
						},
						"required": []string{"prompt"},
					},
					"description": "Advanced mode: List of sequential segments, each with its own prompt and optional reference images.",
				},
				"model": map[string]any{
					"type":        "string",
					"description": "Veo model to use (Veo 3.1 or Veo 3.1 Fast; Lite is not supported for extension)",
					"default":     "veo-3.1-fast-generate-preview",
				},
				"aspect_ratio": map[string]any{
					"type":        "string",
					"description": "Aspect ratio (must match original video)",
					"enum":        []string{"16:9", "9:16"},
				},
				"resolution": map[string]any{
					"type":        "string",
					"description": "Resolution: '720p' or '1080p'",
					"enum":        []string{"720p", "1080p"},
				},
				"duration_seconds": map[string]any{
					"type":        "integer",
					"description": "Duration of the extension in seconds",
				},
				"fps": map[string]any{
					"type":        "integer",
					"description": "Frames per second",
				},
				"negative_prompt": map[string]any{
					"type":        "string",
					"description": "Explicitly state what should not be included",
				},
				"enhance_prompt": map[string]any{
					"type":        "boolean",
					"description": "Whether to use prompt rewriting logic",
				},
				"generate_audio": map[string]any{
					"type":        "boolean",
					"description": "Whether to generate audio",
				},
				"seed": map[string]any{
					"type":        "integer",
					"description": "RNG seed",
				},
				"person_generation": map[string]any{
					"type":        "string",
					"description": "Person generation policy: 'dont_allow' or 'allow_adult'",
					"enum":        []string{"dont_allow", "allow_adult"},
				},
				"reference_images": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"path": map[string]any{
								"type":        "string",
								"description": "Absolute local path to image",
							},
							"type": map[string]any{
								"type":        "string",
								"description": "Reference type: 'ASSET' or 'STYLE'",
							},
						},
						"required": []string{"path"},
					},
					"description": "Global reference images to use as fallback for segments that do not have their own reference_images specified.",
				},
			},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args json.RawMessage) (*mcp.CallToolResult, any, error) {
		var toolArgs GenerateExtendedSequenceArgs
		if err := json.Unmarshal(args, &toolArgs); err != nil {
			return nil, nil, err
		}

		var runSegments []SegmentArgs
		if len(toolArgs.Segments) > 0 {
			runSegments = toolArgs.Segments
		} else if len(toolArgs.Prompts) > 0 {
			for _, p := range toolArgs.Prompts {
				runSegments = append(runSegments, SegmentArgs{Prompt: p})
			}
		} else {
			return formatJSONResult(nil, fmt.Errorf("either prompts or segments array must be provided"))
		}

		model := toolArgs.Model
		if model == "" {
			model = getDefaultModel()
		}

		config := &genai.GenerateVideosConfig{
			AspectRatio:      toolArgs.AspectRatio,
			Resolution:       toolArgs.Resolution,
			DurationSeconds:  toolArgs.DurationSeconds,
			FPS:              toolArgs.FPS,
			NegativePrompt:   toolArgs.NegativePrompt,
			GenerateAudio:    toolArgs.GenerateAudio,
			Seed:             toolArgs.Seed,
			PersonGeneration: toolArgs.PersonGeneration,
		}
		if toolArgs.EnhancePrompt != nil {
			config.EnhancePrompt = *toolArgs.EnhancePrompt
		}

		// Resolve global fallback reference images if any
		globalRefImages, err := resolveReferenceImages(toolArgs.ReferenceImages)
		if err != nil {
			return formatJSONResult(nil, err)
		}

		var segments []VideoGenerationResponse
		var segmentPaths []string

		for i, seg := range runSegments {
			promptStr := strings.TrimSpace(seg.Prompt)
			if promptStr == "" {
				return formatJSONResult(nil, fmt.Errorf("prompt at index %d cannot be empty", i))
			}

			// Model autodetection inside inline prompt
			parsedPrompt, detectedModel := parseModelAndPrompt(promptStr)
			if detectedModel != "" {
				model = detectedModel
				promptStr = parsedPrompt
			}

			// Resolve reference images for this segment (fallback to global if segment specific ones are empty)
			var segmentRefImages []*genai.VideoGenerationReferenceImage
			if i == 0 {
				if len(seg.ReferenceImages) > 0 {
					segmentRefImages, err = resolveReferenceImages(seg.ReferenceImages)
					if err != nil {
						return formatJSONResult(nil, err)
					}
				} else {
					segmentRefImages = globalRefImages
				}
			} else if len(seg.ReferenceImages) > 0 || len(toolArgs.ReferenceImages) > 0 {
				fmt.Fprintf(os.Stderr, "Warning: Reference images are not supported for extended segments (Segment %d+). Skipping reference images for this segment.\n", i+1)
			}

			var source *genai.GenerateVideosSource
			if i == 0 {
				// Base video
				source = &genai.GenerateVideosSource{
					Prompt: promptStr,
				}
			} else {
				// Extend from previous segment's URI
				prevSegment := segments[i-1]
				source = &genai.GenerateVideosSource{
					Prompt: promptStr,
					Video: &genai.Video{
						URI: prevSegment.VideoURI,
					},
				}
			}

			fmt.Fprintf(os.Stderr, "Generating segment %d/%d: %s\n", i+1, len(runSegments), promptStr)
			res, err := generateVideoHelper(ctx, client, model, source, config, segmentRefImages)
			if err != nil {
				// If an extension fails, we return what we generated so far with the error details
				return formatJSONResult(&ExtendedSequenceResponse{
					Segments:        segments,
					CombinedSuccess: false,
					CombinedMessage: fmt.Sprintf("Failed at segment %d: %v", i+1, err),
				}, nil)
			}

			segments = append(segments, *res)
			segmentPaths = append(segmentPaths, res.VideoPath)
		}

		// Try combining videos using ffmpeg
		combinedPath, err := combineVideosFFmpeg(segmentPaths)
		success := err == nil
		msg := "Successfully combined segments into a single video."
		if err != nil {
			msg = fmt.Sprintf("Combining segments skipped or failed: %v", err)
		}

		res := &ExtendedSequenceResponse{
			CombinedVideoPath: combinedPath,
			Segments:          segments,
			CombinedSuccess:   success,
			CombinedMessage:   msg,
		}

		return formatJSONResult(res, nil)
	})

	// --- list_generated_videos ---
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_generated_videos",
		Description: "List all generated videos in the output directory",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args json.RawMessage) (*mcp.CallToolResult, any, error) {
		fmt.Fprintf(os.Stderr, "Listing videos in output directory: %s\n", outputDir)
		res, err := listVideosHelper()
		return formatJSONResult(res, err)
	})

	// --- get_video_info ---
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_video_info",
		Description: "Get detailed information about a video file",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"video_path": map[string]any{
					"type":        "string",
					"description": "Path to the video file (can be absolute or relative to output directory)",
				},
			},
			"required": []string{"video_path"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args json.RawMessage) (*mcp.CallToolResult, any, error) {
		var toolArgs GetVideoInfoArgs
		if err := json.Unmarshal(args, &toolArgs); err != nil {
			return nil, nil, err
		}

		videoPath := strings.TrimSpace(toolArgs.VideoPath)
		if videoPath == "" {
			return formatJSONResult(nil, fmt.Errorf("video_path cannot be empty"))
		}

		resolvedVideoPath, err := resolveSafePath(videoPath)
		if err != nil {
			return formatJSONResult(nil, err)
		}

		res, err := getVideoInfoHelper(resolvedVideoPath)
		return formatJSONResult(res, err)
	})

	// 5. Run Server
	switch *transportType {
	case "stdio":
		fmt.Fprintln(os.Stderr, "Veo 3 MCP Server running on stdio")
		if err := s.Run(ctx, &mcp.StdioTransport{}); err != nil {
			log.Fatalf("Stdio server failed: %v", err)
		}
	case "sse":
		handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
			return s
		}, nil)

		fmt.Fprintf(os.Stderr, "Veo 3 MCP Server running on SSE at %s:%d\n", *host, *port)
		if err := http.ListenAndServe(fmt.Sprintf("%s:%d", *host, *port), handler); err != nil {
			log.Fatalf("SSE server failed: %v", err)
		}
	default:
		log.Fatalf("Unknown transport type: %s", *transportType)
	}
}

// Helper function to resolve paths and avoid directory traversal
func resolveSafePath(userPath string) (string, error) {
	if filepath.IsAbs(userPath) {
		return filepath.Clean(userPath), nil
	}
	absRoot, err := filepath.Abs(outputDir)
	if err != nil {
		return "", err
	}
	resolved := filepath.Join(absRoot, userPath)
	if !strings.HasPrefix(resolved, absRoot) {
		return "", fmt.Errorf("security boundary violation: path escapes allowed output directory")
	}
	return resolved, nil
}

func ValidateAuthentication(cliKey string) (string, error) {
	fmt.Fprintln(os.Stderr, "DEBUG - Validating authentication...")

	if cliKey != "" {
		fmt.Fprintln(os.Stderr, "✓ Found API key from CLI argument")
		return cliKey, nil
	}
	if key := os.Getenv("VEO_GEMINI_API_KEY"); key != "" {
		fmt.Fprintln(os.Stderr, "✓ Found VEO_GEMINI_API_KEY environment variable")
		return key, nil
	}
	if key := os.Getenv("VEO_GOOGLE_API_KEY"); key != "" {
		fmt.Fprintln(os.Stderr, "✓ Found VEO_GOOGLE_API_KEY environment variable")
		return key, nil
	}
	if key := os.Getenv("GEMINI_API_KEY"); key != "" {
		fmt.Fprintln(os.Stderr, "✓ Found GEMINI_API_KEY environment variable")
		return key, nil
	}
	if key := os.Getenv("GOOGLE_API_KEY"); key != "" {
		fmt.Fprintln(os.Stderr, "✓ Found GOOGLE_API_KEY environment variable")
		return key, nil
	}
	if key := os.Getenv("GEMINI_CLI_APP"); key != "" {
		fmt.Fprintln(os.Stderr, "✓ Found GEMINI_CLI_APP environment variable")
		return key, nil
	}
	return "", fmt.Errorf("ERROR: No valid API key found. Please set GEMINI_API_KEY, GEMINI_CLI_APP, or other API key environment variable")
}

func resolveReferenceImages(images []ReferenceImageArgs) ([]*genai.VideoGenerationReferenceImage, error) {
	var refImages []*genai.VideoGenerationReferenceImage
	for _, imgArg := range images {
		path := imgArg.ImagePath
		if path == "" {
			path = imgArg.PathAlias
		}
		path = strings.TrimSpace(path)
		if path == "" {
			return nil, fmt.Errorf("reference image path cannot be empty")
		}
		resolved, err := resolveSafePath(path)
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(resolved); os.IsNotExist(err) {
			return nil, fmt.Errorf("reference image not found: %s", resolved)
		}
		data, err := os.ReadFile(resolved)
		if err != nil {
			return nil, fmt.Errorf("failed to read reference image %s: %w", resolved, err)
		}
		mimeType := "image/jpeg"
		ext := strings.ToLower(filepath.Ext(resolved))
		if ext == ".png" {
			mimeType = "image/png"
		} else if ext == ".webp" {
			mimeType = "image/webp"
		}

		refTypeStr := imgArg.ReferenceType
		if refTypeStr == "" {
			refTypeStr = imgArg.TypeAlias
		}
		refType := genai.VideoGenerationReferenceTypeAsset
		if strings.ToUpper(refTypeStr) == "STYLE" {
			refType = genai.VideoGenerationReferenceTypeStyle
		}

		refImages = append(refImages, &genai.VideoGenerationReferenceImage{
			Image: &genai.Image{
				ImageBytes: data,
				MIMEType:   mimeType,
			},
			ReferenceType: refType,
		})
	}
	return refImages, nil
}

func resolveVideo(videoURI, videoPath string) (*genai.Video, error) {
	if videoURI != "" {
		return &genai.Video{
			URI: videoURI,
		}, nil
	}
	if videoPath != "" {
		resolved, err := resolveSafePath(videoPath)
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(resolved); os.IsNotExist(err) {
			return nil, fmt.Errorf("extension video file not found: %s", resolved)
		}
		data, err := os.ReadFile(resolved)
		if err != nil {
			return nil, fmt.Errorf("failed to read video file %s: %w", resolved, err)
		}
		mimeType := "video/mp4"
		ext := strings.ToLower(filepath.Ext(resolved))
		if ext == ".mov" {
			mimeType = "video/quicktime"
		} else if ext == ".avi" {
			mimeType = "video/x-msvideo"
		} else if ext == ".webm" {
			mimeType = "video/webm"
		}
		return &genai.Video{
			VideoBytes: data,
			MIMEType:   mimeType,
		}, nil
	}
	return nil, nil
}

func generateVideoHelper(ctx context.Context, client *genai.Client, model string, source *genai.GenerateVideosSource, config *genai.GenerateVideosConfig, refImages []*genai.VideoGenerationReferenceImage) (*VideoGenerationResponse, error) {
	startTime := time.Now()
	fmt.Fprintf(os.Stderr, "Starting video generation. Model: %s, Prompt: %s\n", model, source.Prompt)

	if config == nil {
		config = &genai.GenerateVideosConfig{}
	}
	// Developer API (genai.BackendGeminiAPI) does not support the generateAudio parameter.
	// Even if set to false, sending it causes API errors. Always omit it.
	config.GenerateAudio = nil

	if len(refImages) > 0 {
		config.ReferenceImages = refImages
	}

	operation, err := client.Models.GenerateVideosFromSource(ctx, model, source, config)
	if err != nil {
		return nil, fmt.Errorf("generation request failed: %w", err)
	}

	// Poll for completion
	maxPollTime := 15 * time.Minute
	pollInterval := 15 * time.Second

	for !operation.Done {
		if time.Since(startTime) > maxPollTime {
			return nil, fmt.Errorf("video generation timed out after %v", maxPollTime)
		}
		fmt.Fprintf(os.Stderr, "Still generating... Elapsed: %.1fs\n", time.Since(startTime).Seconds())
		time.Sleep(pollInterval)

		operation, err = client.Operations.GetVideosOperation(ctx, operation, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to poll operation status: %w", err)
		}
	}

	if operation.Response == nil || len(operation.Response.GeneratedVideos) == 0 {
		return nil, fmt.Errorf("video generation completed but returned empty results")
	}

	generatedVideo := operation.Response.GeneratedVideos[0]

	// Save file with timestamped name
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("veo3_video_%s.mp4", timestamp)
	savePath := filepath.Join(outputDir, filename)

	fmt.Fprintf(os.Stderr, "Downloading generated video from %s...\n", generatedVideo.Video.URI)
	data, err := client.Files.Download(ctx, genai.NewDownloadURIFromGeneratedVideo(generatedVideo), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to download video: %w", err)
	}

	if err := os.WriteFile(savePath, data, 0644); err != nil {
		return nil, fmt.Errorf("failed to save video to disk at %s: %w", savePath, err)
	}

	fileSize := int64(len(data))
	genTime := time.Since(startTime).Seconds()
	fmt.Fprintf(os.Stderr, "Successfully generated and saved video to %s (%d bytes) in %.1fs\n", savePath, fileSize, genTime)

	var negPrompt *string
	if config.NegativePrompt != "" {
		negPrompt = &config.NegativePrompt
	}

	aspectRatio := "16:9"
	if config.AspectRatio != "" {
		aspectRatio = config.AspectRatio
	}

	return &VideoGenerationResponse{
		VideoPath:      savePath,
		Filename:       filename,
		Model:          model,
		Prompt:         source.Prompt,
		NegativePrompt: negPrompt,
		GenerationTime: genTime,
		FileSize:       fileSize,
		AspectRatio:    aspectRatio,
		VideoURI:       generatedVideo.Video.URI,
	}, nil
}

func listVideosHelper() (*VideoListResponse, error) {
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read output directory: %w", err)
	}

	var list []VideoFileInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == ".mp4" || ext == ".mov" || ext == ".avi" || ext == ".mkv" {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			fullPath := filepath.Join(outputDir, entry.Name())
			list = append(list, VideoFileInfo{
				Filename: entry.Name(),
				Path:     fullPath,
				Size:     info.Size(),
				SizeMB:   float64(info.Size()) / 1024.0 / 1024.0,
				Created:  info.ModTime().Format(time.RFC3339),
				Modified: info.ModTime().Format(time.RFC3339),
			})
		}
	}

	return &VideoListResponse{
		Videos:     list,
		TotalCount: len(list),
		OutputDir:  outputDir,
	}, nil
}

func getVideoInfoHelper(fullPath string) (*VideoInfoResponse, error) {
	info, err := os.Stat(fullPath)
	if err != nil {
		return nil, fmt.Errorf("video file not found: %w", err)
	}

	return &VideoInfoResponse{
		Filename: info.Name(),
		Path:     fullPath,
		Size:     info.Size(),
		Created:  info.ModTime().Format(time.RFC3339),
		Modified: info.ModTime().Format(time.RFC3339),
	}, nil
}

func formatJSONResult(res any, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: err.Error()},
			},
		}, nil, nil
	}

	data, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Error marshaling response: %v", err)},
			},
		}, nil, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
	}, nil, nil
}

func parseModelAndPrompt(prompt string) (string, string) {
	lowerPrompt := strings.ToLower(prompt)

	type modelMapping struct {
		patterns []string
		model    string
	}

	mappings := []modelMapping{
		{
			patterns: []string{"veo-3.1-lite-generate-preview", "veo-3.1-lite", "veo 3.1 lite", "veo3.1 lite", "veo3.1-lite"},
			model:    "veo-3.1-lite-generate-preview",
		},
		{
			patterns: []string{"veo-3.1-fast-generate-preview", "veo-3.1-fast", "veo 3.1 fast", "veo3.1 fast", "veo3.1-fast"},
			model:    "veo-3.1-fast-generate-preview",
		},
		{
			patterns: []string{"veo-3.1-generate-preview", "veo-3.1-standard", "veo-3.1", "veo 3.1 standard", "veo 3.1", "veo3.1 standard", "veo3.1"},
			model:    "veo-3.1-generate-preview",
		},
		{
			patterns: []string{"veo-2.0-generate-001", "veo-2.0", "veo-2", "veo 2.0", "veo 2", "veo2"},
			model:    "veo-2.0-generate-001",
		},
	}

	for _, mapping := range mappings {
		for _, pattern := range mapping.patterns {
			prefixes := []string{
				"using model " + pattern,
				"with model " + pattern,
				"model " + pattern,
				"using " + pattern,
				"with " + pattern,
				"via " + pattern,
				pattern,
			}
			for _, prefix := range prefixes {
				idx := strings.Index(lowerPrompt, prefix)
				if idx != -1 {
					detectedModel := mapping.model
					start := prompt[:idx]
					end := prompt[idx+len(prefix):]
					newPrompt := strings.TrimSpace(start + " " + end)
					for strings.Contains(newPrompt, "  ") {
						newPrompt = strings.ReplaceAll(newPrompt, "  ", " ")
					}
					newPrompt = strings.Trim(newPrompt, " ,.;")
					return newPrompt, detectedModel
				}
			}
		}
	}

	// Fallback to generic pattern detection: "model <any-model>" or "using model <any-model>"
	words := strings.Fields(prompt)
	for i, word := range words {
		lowerWord := strings.ToLower(word)
		if lowerWord == "model" && i+1 < len(words) {
			nextWord := words[i+1]
			cleanModel := strings.Trim(nextWord, " ,.;()'\"")
			if strings.Contains(cleanModel, "veo-") || strings.Contains(cleanModel, "generate") {
				var remaining []string
				remaining = append(remaining, words[:i]...)
				remaining = append(remaining, words[i+2:]...)
				newPrompt := strings.Join(remaining, " ")
				return newPrompt, cleanModel
			}
		}
	}

	return prompt, ""
}

func getDefaultModel() string {
	if m := os.Getenv("VEO_DEFAULT_MODEL"); m != "" {
		return m
	}
	return "veo-3.1-fast-generate-preview"
}

func combineVideosFFmpeg(paths []string) (string, error) {
	if len(paths) == 0 {
		return "", fmt.Errorf("no paths to combine")
	}
	if len(paths) == 1 {
		return paths[0], nil
	}

	// Check if ffmpeg is in PATH
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", fmt.Errorf("ffmpeg not found in PATH")
	}

	// Create temp file for concat demuxer
	tmpFile, err := os.CreateTemp("", "veo_concat_*.txt")
	if err != nil {
		return "", fmt.Errorf("failed to create temp concat file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	for _, path := range paths {
		// Escape single quotes for ffmpeg
		escapedPath := strings.ReplaceAll(path, "'", "'\\''")
		_, err := fmt.Fprintf(tmpFile, "file '%s'\n", escapedPath)
		if err != nil {
			tmpFile.Close()
			return "", fmt.Errorf("failed to write to temp file: %w", err)
		}
	}
	tmpFile.Close()

	// Generate a unique output file name in outputDir
	timestamp := time.Now().Format("20060102_150405")
	outputFilename := fmt.Sprintf("veo3_combined_%s.mp4", timestamp)
	outputPath := filepath.Join(outputDir, outputFilename)

	// Run ffmpeg
	cmd := exec.Command(ffmpegPath, "-y", "-f", "concat", "-safe", "0", "-i", tmpFile.Name(), "-c", "copy", outputPath)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ffmpeg command failed: %w", err)
	}

	return outputPath, nil
}
