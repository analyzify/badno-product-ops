package images

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"
)

// Resizer handles image resizing operations
type Resizer struct {
	inputDir  string
	outputDir string
}

// NewResizer creates a new image resizer
func NewResizer() *Resizer {
	return &Resizer{
		inputDir:  "output/originals",
		outputDir: "output/resized",
	}
}

// FindOriginals returns paths to all original images
func (r *Resizer) FindOriginals() ([]string, error) {
	var images []string

	err := filepath.Walk(r.inputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" {
			images = append(images, path)
		}
		return nil
	})

	return images, err
}

// ResizeSquare resizes an image to a square with center-crop
func (r *Resizer) ResizeSquare(srcPath string, size int) (string, error) {
	// Open source image
	src, err := imaging.Open(srcPath)
	if err != nil {
		return "", err
	}

	// Get dimensions
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Center-crop to square
	var cropped image.Image
	if width > height {
		// Landscape: crop sides
		offset := (width - height) / 2
		cropped = imaging.Crop(src, image.Rect(offset, 0, offset+height, height))
	} else if height > width {
		// Portrait: crop top/bottom
		offset := (height - width) / 2
		cropped = imaging.Crop(src, image.Rect(0, offset, width, offset+width))
	} else {
		// Already square
		cropped = imaging.Clone(src)
	}

	// Resize to target size
	resized := imaging.Resize(cropped, size, size, imaging.Lanczos)

	// Create output directory
	sizeDir := filepath.Join(r.outputDir, fmt.Sprintf("%d", size))
	if err := os.MkdirAll(sizeDir, 0755); err != nil {
		return "", err
	}

	// Save resized image
	filename := filepath.Base(srcPath)
	destPath := filepath.Join(sizeDir, filename)

	if err := imaging.Save(resized, destPath); err != nil {
		return "", err
	}

	return destPath, nil
}
