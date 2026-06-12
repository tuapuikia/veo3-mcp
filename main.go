package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
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

// Args structures for tool inputs
type GenerateVideoArgs struct {
	Prompt string `json:"prompt"`
	Model  string `json:"model,omitempty"`
}

type GenerateVideoFromImageArgs struct {
	Prompt    string `json:"prompt"`
	ImagePath string `json:"image_path"`
	Model     string `json:"model,omitempty"`
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
		home, err := os.UserHomeDir()
		if err == nil {
			outputDir = filepath.Join(home, "Videos", "Generated")
		} else {
			outputDir = "./videos"
		}
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
		Version: "1.0.0",
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
					"description": "Veo model to use (veo-3.0-generate-preview, veo-3.0-fast-generate-preview, veo-2.0-generate-001)",
					"enum":        []string{"veo-3.0-generate-preview", "veo-3.0-fast-generate-preview", "veo-2.0-generate-001"},
					"default":     "veo-3.0-generate-preview",
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
		if model == "" {
			model = "veo-3.0-generate-preview"
		}

		res, err := generateVideoHelper(ctx, client, toolArgs.Prompt, model, nil)
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
					"description": "Veo model to use (veo-3.0-generate-preview, veo-3.0-fast-generate-preview, veo-2.0-generate-001)",
					"enum":        []string{"veo-3.0-generate-preview", "veo-3.0-fast-generate-preview", "veo-2.0-generate-001"},
					"default":     "veo-3.0-generate-preview",
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
		if model == "" {
			model = "veo-3.0-generate-preview"
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

		res, err := generateVideoHelper(ctx, client, toolArgs.Prompt, model, img)
		return formatJSONResult(res, err)
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
	if key := os.Getenv("NANOBANANA_GEMINI_API_KEY"); key != "" {
		fmt.Fprintln(os.Stderr, "✓ Found NANOBANANA_GEMINI_API_KEY environment variable")
		return key, nil
	}
	if key := os.Getenv("NANOBANANA_GOOGLE_API_KEY"); key != "" {
		fmt.Fprintln(os.Stderr, "✓ Found NANOBANANA_GOOGLE_API_KEY environment variable")
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

func generateVideoHelper(ctx context.Context, client *genai.Client, prompt string, model string, img *genai.Image) (*VideoGenerationResponse, error) {
	startTime := time.Now()
	fmt.Fprintf(os.Stderr, "Starting video generation. Model: %s, Prompt: %s\n", model, prompt)

	var operation *genai.GenerateVideosOperation
	var err error

	if img != nil {
		fmt.Fprintln(os.Stderr, "Calling GenerateVideos with image input...")
		operation, err = client.Models.GenerateVideos(ctx, model, prompt, img, nil)
	} else {
		fmt.Fprintln(os.Stderr, "Calling GenerateVideos with text input...")
		operation, err = client.Models.GenerateVideos(ctx, model, prompt, nil, nil)
	}

	if err != nil {
		return nil, fmt.Errorf("generation request failed: %w", err)
	}

	// Poll for completion
	maxPollTime := 10 * time.Minute
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

	return &VideoGenerationResponse{
		VideoPath:      savePath,
		Filename:       filename,
		Model:          model,
		Prompt:         prompt,
		NegativePrompt: nil,
		GenerationTime: genTime,
		FileSize:       fileSize,
		AspectRatio:    "16:9",
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
