package allowed

import "strings"

type MimeList struct {
	Types map[string]string
}

func (m *MimeList) Allowed(contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if idx := strings.Index(contentType, ";"); idx != -1 {
		contentType = strings.TrimSpace(contentType[:idx])
	}

	_, exists := m.Types[contentType]
	return exists
}

func (m *MimeList) Extension(contentType string) string {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if idx := strings.Index(contentType, ";"); idx != -1 {
		contentType = strings.TrimSpace(contentType[:idx])
	}

	if ext, exists := m.Types[contentType]; exists {
		return ext
	}

	if strings.HasPrefix(contentType, "image/") {
		return ".img"
	}
	if strings.HasPrefix(contentType, "video/") {
		return ".video"
	}

	return ".bin"
}

type Config struct {
	AllowImages    bool     `json:"allow_images"`
	AllowVideos    bool     `json:"allow_videos"`
	AllowDocuments bool     `json:"allow_documents"`
	CustomTypes    []string `json:"custom_types"`
	MaxImageSize   int64    `json:"max_image_size"`
	MaxVideoSize   int64    `json:"max_video_size"`
}

func LoadFromConfig(cfg Config) *MimeList {
	types := make(map[string]string)

	if cfg.AllowImages {
		types["image/jpeg"] = ".jpg"
		types["image/png"] = ".png"
		types["image/webp"] = ".webp"
	}

	if cfg.AllowVideos {
		types["video/mp4"] = ".mp4"
		types["video/webm"] = ".webm"
		types["video/quicktime"] = ".mov"
	}

	if cfg.AllowDocuments {
		types["application/pdf"] = ".pdf"
		types["text/plain"] = ".txt"
	}

	for _, customType := range cfg.CustomTypes {
		ext := getDefaultExtension(customType)
		types[customType] = ext
	}

	return &MimeList{Types: types}
}

func getDefaultExtension(mimeType string) string {
	parts := strings.Split(mimeType, "/")
	//nolint:mnd // parts length is fixed
	if len(parts) == 2 {
		return "." + parts[1]
	}
	return ".bin"
}
