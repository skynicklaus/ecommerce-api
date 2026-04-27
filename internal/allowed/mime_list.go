package allowed

import (
	"maps"
	"strings"
	"sync"
)

type MimeList struct {
	Types map[string]string
	mu    sync.RWMutex
}

func NewMimeList() *MimeList {
	//nolint:exhaustruct // no need for mutex
	return &MimeList{
		Types: map[string]string{
			"image/jpeg":    ".jpg",
			"image/png":     ".png",
			"image/gif":     ".gif",
			"image/webp":    ".webp",
			"image/bmp":     ".bmp",
			"image/svg+xml": ".svg",
			"image/tiff":    ".tiff",

			"video/mp4":       ".mp4",
			"video/webm":      ".webm",
			"video/quicktime": ".mov",
			"video/x-msvideo": ".avi",
			"video/mpeg":      ".mpeg",

			"application/pdf": ".pdf",
			"text/plain":      ".txt",
		},
	}
}

func NewMimeListWithTypes(types map[string]string) *MimeList {
	//nolint:exhaustruct // no need for mutex
	return &MimeList{
		Types: types,
	}
}

func (m *MimeList) Allowed(contentType string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if idx := strings.Index(contentType, ";"); idx != -1 {
		contentType = strings.TrimSpace(contentType[:idx])
	}

	if _, exists := m.Types[contentType]; exists {
		return true
	}

	if strings.HasPrefix(contentType, "image/") {
		return m.allowsImagePrefix()
	}
	if strings.HasPrefix(contentType, "video/") {
		return m.allowsVideoPrefix()
	}

	return false
}

func (m *MimeList) Extension(contentType string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Normalize content type
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if idx := strings.Index(contentType, ";"); idx != -1 {
		contentType = strings.TrimSpace(contentType[:idx])
	}

	if ext, exists := m.Types[contentType]; exists {
		return ext
	}

	// Fallback extensions based on type prefix
	if strings.HasPrefix(contentType, "image/") {
		return ".img"
	}
	if strings.HasPrefix(contentType, "video/") {
		return ".video"
	}
	if strings.HasPrefix(contentType, "audio/") {
		return ".audio"
	}

	return ".bin"
}

func (m *MimeList) Add(contentType, extension string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if !strings.HasPrefix(extension, ".") {
		extension = "." + extension
	}

	m.Types[contentType] = extension
}

func (m *MimeList) Remove(contentType string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	contentType = strings.ToLower(strings.TrimSpace(contentType))
	delete(m.Types, contentType)
}

func (m *MimeList) IsImage(contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if idx := strings.Index(contentType, ";"); idx != -1 {
		contentType = strings.TrimSpace(contentType[:idx])
	}
	return strings.HasPrefix(contentType, "image/")
}

func (m *MimeList) IsVideo(contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if idx := strings.Index(contentType, ";"); idx != -1 {
		contentType = strings.TrimSpace(contentType[:idx])
	}
	return strings.HasPrefix(contentType, "video/")
}

func (m *MimeList) IsDocument(contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if idx := strings.Index(contentType, ";"); idx != -1 {
		contentType = strings.TrimSpace(contentType[:idx])
	}

	documentTypes := []string{
		"application/pdf",
		"text/plain",
		"application/msword",
		"application/vnd.openxmlformats-officedocument",
	}

	for _, docType := range documentTypes {
		if strings.HasPrefix(contentType, docType) {
			return true
		}
	}

	return false
}

func (m *MimeList) List() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]string, len(m.Types))
	maps.Copy(result, m.Types)
	return result
}

func (m *MimeList) allowsImagePrefix() bool {
	for mimeType := range m.Types {
		if strings.HasPrefix(mimeType, "image/") {
			return true
		}
	}
	return false
}

func (m *MimeList) allowsVideoPrefix() bool {
	for mimeType := range m.Types {
		if strings.HasPrefix(mimeType, "video/") {
			return true
		}
	}
	return false
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
		types["image/gif"] = ".gif"
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

	//nolint:exhaustruct // no need for mutex
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
