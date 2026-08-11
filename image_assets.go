package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/image/webp"
)

const (
	maxImageAssetSize        = 16 << 20
	maxImageDimension        = 16384
	maxImagePixels     int64 = 64 * 1024 * 1024
	publicImageTimeout       = 20 * time.Second
)

// ImageAsset is returned after an image has been copied next to a local
// Markdown document or uploaded next to a WebDAV document. MarkdownURL is
// always a relative, URL-escaped reference; it never contains a local absolute
// path, a WebDAV endpoint, or credentials.
type ImageAsset struct {
	MarkdownURL string `json:"markdownURL"`
	Name        string `json:"name"`
	MIMEType    string `json:"mimeType"`
	Size        int64  `json:"size"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	SHA256      string `json:"sha256"`
}

// ImageAssetData is the bounded representation used across the Wails bridge
// for file selection and secure preview/export resolution.
type ImageAssetData struct {
	Name       string `json:"name"`
	MIMEType   string `json:"mimeType"`
	DataBase64 string `json:"dataBase64"`
	Size       int64  `json:"size"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	SHA256     string `json:"sha256"`
}

type validatedImage struct {
	data      []byte
	name      string
	mimeType  string
	extension string
	width     int
	height    int
	sha256    string
}

type webDAVImageRepresentationState uint8

const (
	webDAVImageRepresentationUnknown webDAVImageRepresentationState = iota
	webDAVImageRepresentationMissing
	webDAVImageRepresentationLockNull
	webDAVImageRepresentationValid
)

// SelectImageFile uses a native dialog and returns validated bytes. The
// selected path itself is intentionally not exposed to the WebView.
func (a *App) SelectImageFile() (ImageAssetData, error) {
	english := a.currentLocale() == "en"
	title := "选择图片"
	imageFilter := "图片文件 (*.png;*.jpg;*.jpeg;*.gif;*.webp)"
	if english {
		title = "Select Image"
		imageFilter = "Image files (*.png;*.jpg;*.jpeg;*.gif;*.webp)"
	}
	selectedPath, err := runtime.OpenFileDialog(a.currentContext(), runtime.OpenDialogOptions{
		Title: title,
		Filters: []runtime.FileFilter{
			{DisplayName: imageFilter, Pattern: "*.png;*.jpg;*.jpeg;*.gif;*.webp"},
		},
	})
	if err != nil {
		return ImageAssetData{}, fmt.Errorf("打开图片对话框失败: %w", err)
	}
	if strings.TrimSpace(selectedPath) == "" {
		return ImageAssetData{}, nil
	}
	imageData, err := readValidatedImageFile(selectedPath)
	if err != nil {
		return ImageAssetData{}, err
	}
	return imageData.bridgeData(), nil
}

// ValidateImageData applies the same byte, format and dimension limits used by
// file selection and imports. The frontend uses it before a Data URI is ever
// assigned to an <img>, so an embedded image cannot bypass native validation.
func (a *App) ValidateImageData(name string, mimeType string, dataBase64 string) (ImageAssetData, error) {
	imageData, err := decodeAndValidateImage(name, mimeType, dataBase64)
	if err != nil {
		return ImageAssetData{}, err
	}
	return imageData.bridgeData(), nil
}

// ImportLocalImageData atomically stores a content-addressed image in
// <document-name>.assets beside an existing local Markdown document.
func (a *App) ImportLocalImageData(documentPath string, name string, mimeType string, dataBase64 string) (ImageAsset, error) {
	imageData, err := decodeAndValidateImage(name, mimeType, dataBase64)
	if err != nil {
		return ImageAsset{}, err
	}
	root, assetDirectory, err := openLocalDocumentRoot(documentPath)
	if err != nil {
		return ImageAsset{}, err
	}
	defer root.Close()
	if err := ensureLocalAssetDirectory(root, assetDirectory); err != nil {
		return ImageAsset{}, err
	}
	assetRoot, err := root.OpenRoot(assetDirectory)
	if err != nil {
		return ImageAsset{}, fmt.Errorf("打开图片资源目录失败: %w", err)
	}
	defer assetRoot.Close()
	if err := verifyRootDirectory(root, assetDirectory, assetRoot); err != nil {
		return ImageAsset{}, err
	}

	filename := imageData.sha256 + imageData.extension
	if err := writeContentAddressedImage(assetRoot, filename, imageData); err != nil {
		return ImageAsset{}, err
	}
	markdownPath := path.Join(filepath.ToSlash(assetDirectory), filename)
	return imageData.bridgeAsset(escapeMarkdownImagePath(markdownPath), filename), nil
}

// ImportWebDAVImageData creates a content-addressed image beside a remote
// document. The opaque document capability, rather than a caller supplied
// remote path, determines the destination.
func (a *App) ImportWebDAVImageData(workspaceID string, remoteDocumentID string, name string, mimeType string, dataBase64 string) (asset ImageAsset, err error) {
	defer func() { err = exposeWebDAVBridgeError(err) }()
	imageData, err := decodeAndValidateImage(name, mimeType, dataBase64)
	if err != nil {
		return ImageAsset{}, err
	}
	capability, err := a.webDAVCapabilityByID(workspaceID)
	if err != nil {
		return ImageAsset{}, err
	}
	remoteDocumentID = strings.TrimSpace(remoteDocumentID)
	capability.mu.Lock()
	defer capability.mu.Unlock()
	if capability.closed || capability.client == nil {
		return ImageAsset{}, closedWebDAVCapabilityError("upload image")
	}
	document, ok := capability.documents[remoteDocumentID]
	if !ok || remoteDocumentID == "" {
		return ImageAsset{}, &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "upload image", Err: errors.New("remote document capability is invalid")}
	}
	assetDirectory, markdownDirectory, err := webDAVDocumentAssetDirectory(document.path)
	if err != nil {
		return ImageAsset{}, err
	}
	ctx := appWebDAVContext(a)
	if err := ensureWebDAVDirectory(ctx, capability.client, assetDirectory); err != nil {
		return ImageAsset{}, err
	}
	filename := imageData.sha256 + imageData.extension
	targetPath := path.Join(assetDirectory, filename)
	if _, err := capability.client.PutImageCreateOnly(ctx, targetPath, imageData); err != nil {
		return ImageAsset{}, err
	}
	return imageData.bridgeAsset(escapeMarkdownImagePath(path.Join(markdownDirectory, filename)), filename), nil
}

// ResolveLocalImage resolves a relative Markdown image URL beneath the local
// document directory. Absolute paths, traversal, and every symlink component
// are rejected.
func (a *App) ResolveLocalImage(documentPath string, source string) (ImageAssetData, error) {
	root, _, err := openLocalDocumentRoot(documentPath)
	if err != nil {
		return ImageAssetData{}, err
	}
	defer root.Close()
	relativePath, err := normalizeRelativeImageSource(source)
	if err != nil {
		return ImageAssetData{}, err
	}
	if err := rejectImagePathSymlinks(root, relativePath); err != nil {
		return ImageAssetData{}, err
	}
	file, err := openRootReadOnlyNonBlocking(root, relativePath)
	if err != nil {
		return ImageAssetData{}, fmt.Errorf("打开本地图片失败: %w", err)
	}
	defer file.Close()
	imageData, err := readValidatedImage(file, path.Base(relativePath), "")
	if err != nil {
		return ImageAssetData{}, err
	}
	return imageData.bridgeData(), nil
}

// ResolveWebDAVImage resolves a relative image URL against the path held by an
// opaque remote-document capability and retrieves it with that workspace's
// in-memory authentication session.
func (a *App) ResolveWebDAVImage(workspaceID string, remoteDocumentID string, source string) (data ImageAssetData, err error) {
	defer func() { err = exposeWebDAVBridgeError(err) }()
	capability, err := a.webDAVCapabilityByID(workspaceID)
	if err != nil {
		return ImageAssetData{}, err
	}
	remoteDocumentID = strings.TrimSpace(remoteDocumentID)
	capability.mu.RLock()
	defer capability.mu.RUnlock()
	if capability.closed || capability.client == nil {
		return ImageAssetData{}, closedWebDAVCapabilityError("read image")
	}
	document, ok := capability.documents[remoteDocumentID]
	if !ok || remoteDocumentID == "" {
		return ImageAssetData{}, &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "read image", Err: errors.New("remote document capability is invalid")}
	}
	relativeSource, err := normalizeRelativeImageSource(source)
	if err != nil {
		return ImageAssetData{}, &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "read image", Err: err}
	}
	target := path.Join(parentWebDAVPath(document.path), relativeSource)
	target, err = normalizeNonRootWebDAVPath(target)
	if err != nil {
		return ImageAssetData{}, invalidWebDAVPathError("read image", target, err)
	}
	imageData, err := capability.client.ReadImage(appWebDAVContext(a), target)
	if err != nil {
		return ImageAssetData{}, err
	}
	return imageData.bridgeData(), nil
}

// FetchPublicImage safely retrieves a public HTTPS image for preview/export.
// It uses a direct, DNS-pinned connection, rejects non-public addresses, and
// permits redirects only within the original HTTPS origin.
func (a *App) FetchPublicImage(source string) (ImageAssetData, error) {
	imageData, err := fetchPublicImage(appWebDAVContext(a), source)
	if err != nil {
		return ImageAssetData{}, err
	}
	return imageData.bridgeData(), nil
}

func (imageData validatedImage) bridgeAsset(markdownURL string, name string) ImageAsset {
	return ImageAsset{
		MarkdownURL: markdownURL,
		Name:        name,
		MIMEType:    imageData.mimeType,
		Size:        int64(len(imageData.data)),
		Width:       imageData.width,
		Height:      imageData.height,
		SHA256:      imageData.sha256,
	}
}

func (imageData validatedImage) bridgeData() ImageAssetData {
	return ImageAssetData{
		Name:       imageData.name,
		MIMEType:   imageData.mimeType,
		DataBase64: base64.StdEncoding.EncodeToString(imageData.data),
		Size:       int64(len(imageData.data)),
		Width:      imageData.width,
		Height:     imageData.height,
		SHA256:     imageData.sha256,
	}
}

func decodeAndValidateImage(name string, declaredMIME string, encoded string) (validatedImage, error) {
	if len(encoded) > base64.StdEncoding.EncodedLen(maxImageAssetSize)+8 {
		return validatedImage{}, imageSizeError()
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return validatedImage{}, errors.New("图片数据不是有效的 Base64")
	}
	return validateImage(data, name, declaredMIME)
}

func readValidatedImageFile(filename string) (validatedImage, error) {
	absolute, err := filepath.Abs(filename)
	if err != nil {
		return validatedImage{}, fmt.Errorf("解析图片路径失败: %w", err)
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return validatedImage{}, fmt.Errorf("读取图片信息失败: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return validatedImage{}, errors.New("只能选择普通图片文件，不允许符号链接")
	}
	file, err := openReadOnlyNonBlocking(absolute)
	if err != nil {
		return validatedImage{}, fmt.Errorf("打开图片失败: %w", err)
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		return validatedImage{}, errors.New("图片文件在读取前发生了变化")
	}
	return readValidatedImage(file, filepath.Base(absolute), "")
}

func readValidatedImage(reader io.Reader, name string, declaredMIME string) (validatedImage, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxImageAssetSize+1))
	if err != nil {
		return validatedImage{}, fmt.Errorf("读取图片失败: %w", err)
	}
	return validateImage(data, name, declaredMIME)
}

func validateImage(data []byte, name string, declaredMIME string) (validatedImage, error) {
	if len(data) == 0 {
		return validatedImage{}, errors.New("图片数据为空")
	}
	if len(data) > maxImageAssetSize {
		return validatedImage{}, imageSizeError()
	}
	mimeType, extension, config, err := decodeImageConfig(data)
	if err != nil {
		return validatedImage{}, err
	}
	if err := validateDeclaredImageMIME(declaredMIME, mimeType); err != nil {
		return validatedImage{}, err
	}
	if err := rejectAnimatedImage(data, mimeType); err != nil {
		return validatedImage{}, err
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > maxImageDimension || config.Height > maxImageDimension || int64(config.Width)*int64(config.Height) > maxImagePixels {
		return validatedImage{}, fmt.Errorf("图片尺寸超过限制（最大边长 %d，最多 %d 百万像素）", maxImageDimension, maxImagePixels/1_000_000)
	}
	digest := sha256.Sum256(data)
	return validatedImage{
		data:      append([]byte(nil), data...),
		name:      safeImageName(name, "image"+extension),
		mimeType:  mimeType,
		extension: extension,
		width:     config.Width,
		height:    config.Height,
		sha256:    hex.EncodeToString(digest[:]),
	}, nil
}

func rejectAnimatedImage(data []byte, mimeType string) error {
	switch mimeType {
	case "image/png":
		if animated, err := isAnimatedPNG(data); err != nil {
			return errors.New("PNG 图片结构无效")
		} else if animated {
			return errors.New("不支持 APNG 动画；请使用静态 PNG")
		}
	case "image/gif":
		frames, err := countGIFFrames(data)
		if err != nil {
			return errors.New("GIF 图片结构无效")
		}
		if frames != 1 {
			return errors.New("不支持动画 GIF；请使用单帧图片")
		}
	case "image/webp":
		if animated, err := isAnimatedWebP(data); err != nil {
			return errors.New("WebP 图片结构无效")
		} else if animated {
			return errors.New("不支持动画 WebP；请使用静态图片")
		}
	}
	return nil
}

func isAnimatedPNG(data []byte) (bool, error) {
	if len(data) < 20 || !bytes.Equal(data[:8], []byte{'\x89', 'P', 'N', 'G', '\r', '\n', '\x1a', '\n'}) {
		return false, errors.New("invalid PNG header")
	}
	for offset := 8; offset < len(data); {
		if offset+12 > len(data) {
			return false, errors.New("truncated PNG chunk")
		}
		chunkSize := uint64(binary.BigEndian.Uint32(data[offset : offset+4]))
		next := uint64(offset) + 12 + chunkSize
		if next > uint64(len(data)) {
			return false, errors.New("truncated PNG chunk payload")
		}
		chunkType := data[offset+4 : offset+8]
		chunkDataEnd := uint64(offset) + 8 + chunkSize
		expectedCRC := binary.BigEndian.Uint32(data[chunkDataEnd:next])
		if crc32.ChecksumIEEE(data[offset+4:chunkDataEnd]) != expectedCRC {
			return false, errors.New("invalid PNG chunk checksum")
		}
		if bytes.Equal(chunkType, []byte("acTL")) || bytes.Equal(chunkType, []byte("fcTL")) || bytes.Equal(chunkType, []byte("fdAT")) {
			return true, nil
		}
		if bytes.Equal(chunkType, []byte("IEND")) {
			if chunkSize != 0 || next != uint64(len(data)) {
				return false, errors.New("invalid PNG trailer")
			}
			return false, nil
		}
		offset = int(next)
	}
	return false, errors.New("PNG trailer is missing")
}

// countGIFFrames walks block boundaries without decompressing pixel data, so
// a many-frame compression bomb is rejected before reaching the WebView.
func countGIFFrames(data []byte) (int, error) {
	if len(data) < 13 || !(bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a"))) {
		return 0, errors.New("invalid GIF header")
	}
	offset := 13
	packed := data[10]
	if packed&0x80 != 0 {
		offset += 3 * (1 << ((packed & 0x07) + 1))
	}
	if offset > len(data) {
		return 0, errors.New("truncated GIF color table")
	}
	frames := 0
	for offset < len(data) {
		switch data[offset] {
		case 0x3b:
			if frames == 0 {
				return 0, errors.New("GIF has no image frame")
			}
			return frames, nil
		case 0x2c:
			frames++
			if frames > 1 {
				return frames, nil
			}
			if offset+10 > len(data) {
				return 0, errors.New("truncated GIF image descriptor")
			}
			localPacked := data[offset+9]
			offset += 10
			if localPacked&0x80 != 0 {
				offset += 3 * (1 << ((localPacked & 0x07) + 1))
			}
			if offset >= len(data) {
				return 0, errors.New("truncated GIF image data")
			}
			offset++ // LZW minimum code size.
		case 0x21:
			if offset+2 > len(data) {
				return 0, errors.New("truncated GIF extension")
			}
			offset += 2 // Extension introducer and label.
		default:
			return 0, errors.New("invalid GIF block")
		}
		for {
			if offset >= len(data) {
				return 0, errors.New("truncated GIF sub-block")
			}
			size := int(data[offset])
			offset++
			if size == 0 {
				break
			}
			if size > len(data)-offset {
				return 0, errors.New("truncated GIF sub-block payload")
			}
			offset += size
		}
	}
	return 0, errors.New("GIF trailer is missing")
}

func isAnimatedWebP(data []byte) (bool, error) {
	if len(data) < 12 || !bytes.Equal(data[:4], []byte("RIFF")) || !bytes.Equal(data[8:12], []byte("WEBP")) {
		return false, errors.New("invalid WebP header")
	}
	declaredSize := uint64(binary.LittleEndian.Uint32(data[4:8])) + 8
	if declaredSize > uint64(len(data)) || declaredSize < 12 {
		return false, errors.New("invalid WebP RIFF size")
	}
	for offset := 12; uint64(offset)+8 <= declaredSize; {
		chunkSize := uint64(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		if bytes.Equal(data[offset:offset+4], []byte("ANIM")) || bytes.Equal(data[offset:offset+4], []byte("ANMF")) {
			return true, nil
		}
		if bytes.Equal(data[offset:offset+4], []byte("VP8X")) {
			if chunkSize < 1 || uint64(offset)+9 > declaredSize {
				return false, errors.New("truncated WebP extended header")
			}
			if data[offset+8]&0x02 != 0 {
				return true, nil
			}
		}
		next := uint64(offset) + 8 + chunkSize + chunkSize%2
		if next > declaredSize || next > uint64(len(data)) {
			return false, errors.New("truncated WebP chunk")
		}
		offset = int(next)
	}
	return false, nil
}

func decodeImageConfig(data []byte) (string, string, image.Config, error) {
	reader := bytes.NewReader(data)
	switch {
	case bytes.HasPrefix(data, []byte{'\x89', 'P', 'N', 'G', '\r', '\n', '\x1a', '\n'}):
		config, err := png.DecodeConfig(reader)
		return decodedImageConfig("image/png", ".png", config, err)
	case len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff:
		config, err := jpeg.DecodeConfig(reader)
		return decodedImageConfig("image/jpeg", ".jpg", config, err)
	case bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a")):
		config, err := gif.DecodeConfig(reader)
		return decodedImageConfig("image/gif", ".gif", config, err)
	case len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		config, err := webp.DecodeConfig(reader)
		return decodedImageConfig("image/webp", ".webp", config, err)
	default:
		return "", "", image.Config{}, errors.New("仅支持有效的 PNG、JPEG、GIF 或 WebP 图片；SVG 不受支持")
	}
}

func decodedImageConfig(mimeType string, extension string, config image.Config, err error) (string, string, image.Config, error) {
	if err != nil {
		return "", "", image.Config{}, errors.New("图片内容损坏或格式不完整")
	}
	return mimeType, extension, config, nil
}

func validateDeclaredImageMIME(declared string, detected string) error {
	declared = strings.TrimSpace(declared)
	if declared == "" {
		return nil
	}
	mediaType, _, err := mime.ParseMediaType(declared)
	if err != nil {
		return errors.New("图片 MIME 类型无效")
	}
	mediaType = strings.ToLower(mediaType)
	if mediaType == "image/jpg" {
		mediaType = "image/jpeg"
	}
	if mediaType != detected {
		return fmt.Errorf("图片声明类型 %q 与实际格式 %q 不一致", mediaType, detected)
	}
	return nil
}

func safeImageName(name string, fallback string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." || name == string(filepath.Separator) || len(name) > 255 || !utf8.ValidString(name) {
		return fallback
	}
	for _, character := range name {
		if character == 0 || character < 0x20 || character == 0x7f {
			return fallback
		}
	}
	return name
}

func imageSizeError() error {
	return fmt.Errorf("图片超过 %d MB 限制", maxImageAssetSize>>20)
}

func openLocalDocumentRoot(documentPath string) (*os.Root, string, error) {
	absolute, err := filepath.Abs(strings.TrimSpace(documentPath))
	if err != nil || strings.TrimSpace(documentPath) == "" {
		return nil, "", errors.New("本地 Markdown 文档路径无效")
	}
	if !isMarkdownFilename(absolute) {
		return nil, "", errors.New("图片资源只能关联到 Markdown 文档")
	}
	documentInfo, err := os.Lstat(absolute)
	if err != nil {
		return nil, "", fmt.Errorf("读取 Markdown 文档信息失败: %w", err)
	}
	if documentInfo.Mode()&os.ModeSymlink != 0 || !documentInfo.Mode().IsRegular() {
		return nil, "", errors.New("Markdown 文档必须是普通文件，不允许符号链接")
	}
	parent := filepath.Dir(absolute)
	root, err := os.OpenRoot(parent)
	if err != nil {
		return nil, "", fmt.Errorf("打开 Markdown 文档目录失败: %w", err)
	}
	rootDocumentInfo, err := root.Lstat(filepath.Base(absolute))
	if err != nil || rootDocumentInfo.Mode()&os.ModeSymlink != 0 || !rootDocumentInfo.Mode().IsRegular() || !os.SameFile(documentInfo, rootDocumentInfo) {
		root.Close()
		return nil, "", errors.New("Markdown 文档在操作前发生了变化")
	}
	stem := strings.TrimSuffix(filepath.Base(absolute), filepath.Ext(absolute))
	if stem == "" || stem == "." || stem == ".." {
		root.Close()
		return nil, "", errors.New("Markdown 文档名称无法创建图片资源目录")
	}
	return root, stem + ".assets", nil
}

func ensureLocalAssetDirectory(root *os.Root, directory string) error {
	info, err := root.Lstat(directory)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("图片资源目录不是安全的普通目录")
		}
		return nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("读取图片资源目录失败: %w", err)
	}
	if err := root.Mkdir(directory, 0o755); err != nil {
		if info, statErr := root.Lstat(directory); statErr == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			return nil
		}
		return fmt.Errorf("创建图片资源目录失败: %w", err)
	}
	return nil
}

func verifyRootDirectory(parent *os.Root, name string, child *os.Root) error {
	parentInfo, err := parent.Lstat(name)
	if err != nil || parentInfo.Mode()&os.ModeSymlink != 0 || !parentInfo.IsDir() {
		return errors.New("图片资源目录已被替换")
	}
	childInfo, err := child.Stat(".")
	if err != nil || !childInfo.IsDir() || !os.SameFile(parentInfo, childInfo) {
		return errors.New("图片资源目录已被替换")
	}
	return nil
}

func writeContentAddressedImage(root *os.Root, filename string, imageData validatedImage) error {
	if existing, err := readImageAtRoot(root, filename); err == nil {
		if existing.sha256 == imageData.sha256 && bytes.Equal(existing.data, imageData.data) {
			return nil
		}
		return errors.New("同名图片资源内容冲突")
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	temporaryID, err := newOpaqueID()
	if err != nil {
		return errors.New("创建图片临时文件标识失败")
	}
	temporaryName := ".inkmark-image-" + temporaryID + ".tmp"
	temporary, err := root.OpenFile(temporaryName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("创建图片临时文件失败: %w", err)
	}
	keepTemporary := true
	defer func() {
		if keepTemporary {
			_ = root.Remove(temporaryName)
		}
	}()
	if _, err := temporary.Write(imageData.data); err != nil {
		temporary.Close()
		return fmt.Errorf("写入图片临时文件失败: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("同步图片临时文件失败: %w", err)
	}
	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()
		return fmt.Errorf("设置图片文件权限失败: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("关闭图片临时文件失败: %w", err)
	}
	if err := root.Link(temporaryName, filename); err != nil {
		if existing, readErr := readImageAtRoot(root, filename); readErr == nil && existing.sha256 == imageData.sha256 && bytes.Equal(existing.data, imageData.data) {
			_ = root.Remove(temporaryName)
			keepTemporary = false
			return nil
		}
		return fmt.Errorf("提交图片资源失败: %w", err)
	}
	if err := root.Remove(temporaryName); err != nil {
		return fmt.Errorf("清理图片临时文件失败: %w", err)
	}
	keepTemporary = false
	written, err := readImageAtRoot(root, filename)
	if err != nil || written.sha256 != imageData.sha256 || !bytes.Equal(written.data, imageData.data) {
		_ = root.Remove(filename)
		return errors.New("图片资源写入后校验失败")
	}
	return nil
}

func readImageAtRoot(root *os.Root, filename string) (validatedImage, error) {
	info, err := root.Lstat(filename)
	if err != nil {
		return validatedImage{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return validatedImage{}, errors.New("图片资源不是安全的普通文件")
	}
	file, err := openRootReadOnlyNonBlocking(root, filename)
	if err != nil {
		return validatedImage{}, err
	}
	defer file.Close()
	return readValidatedImage(file, path.Base(filename), "")
}

func rejectImagePathSymlinks(root *os.Root, relativePath string) error {
	current := ""
	for _, component := range strings.Split(relativePath, "/") {
		current = path.Join(current, component)
		info, err := root.Lstat(current)
		if err != nil {
			return fmt.Errorf("读取本地图片路径失败: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("本地图片路径不允许符号链接")
		}
	}
	return nil
}

func normalizeRelativeImageSource(source string) (string, error) {
	source = strings.TrimSpace(source)
	if source == "" || len(source) > maxWebDAVPathLength || !utf8.ValidString(source) {
		return "", errors.New("图片相对路径无效")
	}
	parsed, err := url.Parse(source)
	if err != nil || parsed.IsAbs() || parsed.Scheme != "" || parsed.Host != "" || parsed.User != nil || parsed.Opaque != "" || parsed.RawQuery != "" || parsed.ForceQuery {
		return "", errors.New("图片必须使用不含查询参数的相对路径")
	}
	escaped := parsed.EscapedPath()
	if strings.Contains(strings.ToLower(escaped), "%2f") || strings.Contains(strings.ToLower(escaped), "%5c") || strings.Contains(strings.ToLower(escaped), "%00") {
		return "", errors.New("图片路径包含不明确的编码分隔符")
	}
	decoded, err := url.PathUnescape(escaped)
	if err != nil || decoded == "" || strings.HasPrefix(decoded, "/") || strings.Contains(decoded, "\\") {
		return "", errors.New("图片相对路径无效")
	}
	for _, character := range decoded {
		if character == 0 || character < 0x20 || character == 0x7f {
			return "", errors.New("图片路径包含控制字符")
		}
	}
	for _, segment := range strings.Split(decoded, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", errors.New("图片路径包含空段或目录穿越")
		}
	}
	if !fs.ValidPath(decoded) {
		return "", errors.New("图片相对路径无效")
	}
	return decoded, nil
}

func escapeMarkdownImagePath(relativePath string) string {
	segments := strings.Split(relativePath, "/")
	for index := range segments {
		segments[index] = url.PathEscape(segments[index])
	}
	return strings.Join(segments, "/")
}

func webDAVDocumentAssetDirectory(documentPath string) (string, string, error) {
	documentPath, err := normalizeWebDAVMarkdownPath(documentPath)
	if err != nil {
		return "", "", invalidWebDAVPathError("upload image", documentPath, err)
	}
	base := path.Base(documentPath)
	stem := strings.TrimSuffix(base, path.Ext(base))
	if stem == "" || stem == "." || stem == ".." {
		return "", "", &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "upload image", Err: errors.New("remote document name cannot own an asset directory")}
	}
	markdownDirectory := stem + ".assets"
	return path.Join(parentWebDAVPath(documentPath), markdownDirectory), markdownDirectory, nil
}

func ensureWebDAVDirectory(ctx context.Context, client *WebDAVClient, directory string) error {
	exists, err := client.webDAVDirectoryExists(ctx, directory)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if err := client.CreateDirectory(ctx, directory); err != nil {
		if exists, verifyErr := client.webDAVDirectoryExists(ctx, directory); verifyErr == nil && exists {
			return nil
		}
		return err
	}
	exists, err = client.webDAVDirectoryExists(ctx, directory)
	if err != nil {
		return err
	}
	if !exists {
		return &WebDAVError{Kind: WebDAVErrorProtocol, Operation: "create image directory", Path: directory, Err: errors.New("created collection could not be verified")}
	}
	return nil
}

func (client *WebDAVClient) webDAVDirectoryExists(ctx context.Context, relativePath string) (bool, error) {
	normalized, err := normalizeNonRootWebDAVPath(relativePath)
	if err != nil {
		return false, invalidWebDAVPathError("stat image directory", relativePath, err)
	}
	if err := client.ensureAdvertisedRoot(ctx); err != nil {
		return false, err
	}
	multiStatus, err := client.propfindImageDirectory(ctx, normalized)
	if err != nil {
		if IsWebDAVErrorKind(err, WebDAVErrorNotFound) {
			return false, nil
		}
		return false, err
	}
	for _, response := range multiStatus.Responses {
		mapped, hrefErr := client.relativePathFromHrefForRequest(response.Href, normalized, true)
		if hrefErr != nil || mapped != normalized {
			continue
		}
		status := webDAVStatusLineCode(response.Status)
		if status == http.StatusNotFound || status == http.StatusGone {
			return false, nil
		}
		if status >= 400 {
			return false, &WebDAVError{Kind: webDAVStatusErrorKind(status), Operation: "stat image directory", Path: normalized, StatusCode: status}
		}
		properties, ok := response.successfulProperties()
		if !ok {
			return false, &WebDAVError{Kind: WebDAVErrorProtocol, Operation: "stat image directory", Path: normalized, Err: errors.New("Depth-0 response did not include successful properties")}
		}
		if properties.ResourceType.Collection == nil {
			return false, &WebDAVError{Kind: WebDAVErrorConflict, Operation: "stat image directory", Path: normalized, Err: errors.New("image asset path is not a collection")}
		}
		return true, nil
	}
	return false, &WebDAVError{Kind: WebDAVErrorProtocol, Operation: "stat image directory", Path: normalized, Err: errors.New("Depth-0 response did not describe the collection")}
}

func (client *WebDAVClient) propfindImageDirectory(ctx context.Context, relativePath string) (webDAVMultiStatus, error) {
	body := strings.NewReader(`<?xml version="1.0" encoding="utf-8"?>
<d:propfind xmlns:d="DAV:"><d:prop><d:resourcetype/></d:prop></d:propfind>`)
	headers := make(http.Header)
	headers.Set("Depth", "0")
	headers.Set("Content-Type", "application/xml; charset=utf-8")
	response, err := client.request(ctx, "PROPFIND", relativePath, true, body, headers)
	if err != nil {
		return webDAVMultiStatus{}, err
	}
	defer response.Body.Close()
	if err := requireWebDAVStatus(response, "stat image directory", relativePath, http.StatusMultiStatus); err != nil {
		drainWebDAVResponse(response.Body)
		return webDAVMultiStatus{}, err
	}
	payload, err := readLimitedWebDAVBody(response.Body, maxWebDAVMultiStatusSize)
	if err != nil {
		return webDAVMultiStatus{}, bodyWebDAVError("stat image directory", relativePath, err)
	}
	var multiStatus webDAVMultiStatus
	decoder := xml.NewDecoder(bytes.NewReader(payload))
	decoder.Strict = true
	if err := decoder.Decode(&multiStatus); err != nil {
		if errors.Is(err, errWebDAVTooManyResponses) {
			return webDAVMultiStatus{}, &WebDAVError{Kind: WebDAVErrorTooLarge, Operation: "stat image directory", Path: relativePath, Err: err}
		}
		return webDAVMultiStatus{}, &WebDAVError{Kind: WebDAVErrorProtocol, Operation: "stat image directory", Path: relativePath, Err: errors.New("invalid multistatus XML")}
	}
	return multiStatus, nil
}

// ReadImage retrieves and validates a bounded binary image. It deliberately
// does not expose a generic authenticated GET primitive.
func (client *WebDAVClient) ReadImage(ctx context.Context, relativePath string) (validatedImage, error) {
	normalized, err := normalizeNonRootWebDAVPath(relativePath)
	if err != nil {
		return validatedImage{}, invalidWebDAVPathError("read image", relativePath, err)
	}
	return client.readImageValidated(ctx, normalized, "read image")
}

func (client *WebDAVClient) readImageValidated(ctx context.Context, normalized string, operation string) (validatedImage, error) {
	imageData, _, err := client.readImageRepresentation(ctx, normalized, operation)
	return imageData, err
}

func (client *WebDAVClient) readImageRepresentation(ctx context.Context, normalized string, operation string) (validatedImage, webDAVImageRepresentationState, error) {
	if err := client.ensureAdvertisedRoot(ctx); err != nil {
		return validatedImage{}, webDAVImageRepresentationUnknown, err
	}
	response, err := client.request(ctx, http.MethodGet, normalized, false, nil, nil)
	if err != nil {
		return validatedImage{}, webDAVImageRepresentationUnknown, err
	}
	defer response.Body.Close()
	if err := requireWebDAVStatus(response, operation, normalized, http.StatusOK); err != nil {
		drainWebDAVResponse(response.Body)
		if IsWebDAVErrorKind(err, WebDAVErrorNotFound) {
			return validatedImage{}, webDAVImageRepresentationMissing, err
		}
		return validatedImage{}, webDAVImageRepresentationUnknown, err
	}
	if response.ContentLength > maxImageAssetSize {
		return validatedImage{}, webDAVImageRepresentationUnknown, &WebDAVError{Kind: WebDAVErrorTooLarge, Operation: operation, Path: normalized, Err: imageSizeError()}
	}
	payload, err := readLimitedWebDAVBody(response.Body, maxImageAssetSize)
	if err != nil {
		return validatedImage{}, webDAVImageRepresentationUnknown, bodyWebDAVError(operation, normalized, err)
	}
	if len(payload) == 0 {
		return validatedImage{}, webDAVImageRepresentationLockNull, &WebDAVError{Kind: WebDAVErrorProtocol, Operation: operation, Path: normalized, Err: errors.New("resource is an empty DAV lock-null representation")}
	}
	imageData, err := validateImage(payload, path.Base(normalized), "")
	if err != nil {
		return validatedImage{}, webDAVImageRepresentationUnknown, &WebDAVError{Kind: WebDAVErrorProtocol, Operation: operation, Path: normalized, Err: err}
	}
	return imageData, webDAVImageRepresentationValid, nil
}

// PutImageCreateOnly performs a lock-null create, writes the bytes under that
// exclusive lock, and compares the complete GET representation before the
// resource is accepted. Existing content-addressed images are reused only
// after an exact byte comparison.
func (client *WebDAVClient) PutImageCreateOnly(ctx context.Context, relativePath string, imageData validatedImage) (WebDAVWriteResult, error) {
	normalized, err := normalizeNonRootWebDAVPath(relativePath)
	if err != nil {
		return WebDAVWriteResult{}, invalidWebDAVPathError("upload image", relativePath, err)
	}
	if validated, validationErr := validateImage(imageData.data, imageData.name, imageData.mimeType); validationErr != nil || validated.sha256 != imageData.sha256 {
		if validationErr == nil {
			validationErr = errors.New("image digest changed")
		}
		return WebDAVWriteResult{}, &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "upload image", Path: normalized, Err: validationErr}
	}
	if err := client.ensureAdvertisedRoot(ctx); err != nil {
		return WebDAVWriteResult{}, err
	}
	lockToken, lockNullCreated, locked, err := client.tryExclusiveWriteLock(ctx, normalized)
	if err != nil {
		return WebDAVWriteResult{}, err
	}
	if !locked {
		return WebDAVWriteResult{}, &WebDAVError{Kind: WebDAVErrorUnsupported, Operation: "upload image", Path: normalized, Err: errors.New("server does not support safe lock-null image creation")}
	}
	defer client.bestEffortUnlock(normalized, lockToken)
	if !lockNullCreated {
		existing, readErr := client.readImageValidated(ctx, normalized, "verify existing image")
		if readErr == nil && existing.sha256 == imageData.sha256 && bytes.Equal(existing.data, imageData.data) {
			return WebDAVWriteResult{Path: normalized, Created: false, ConcurrencyMode: "dav-lock-existing"}, nil
		}
		if readErr != nil {
			return WebDAVWriteResult{}, readErr
		}
		return WebDAVWriteResult{}, &WebDAVError{Kind: WebDAVErrorConflict, Operation: "upload image", Path: normalized, Err: errors.New("remote image path already contains different data")}
	}

	// A lock-null resource may be deleted only when a readback proves that no
	// representation was committed. A 2xx PUT is already committed even when
	// the subsequent GET is unavailable, and a transport error is ambiguous
	// until readback says otherwise. Preserving an uncertain representation is
	// safer than deleting a successful upload.
	deleteLockNull := false
	defer func() {
		if deleteLockNull {
			client.bestEffortDeleteLocked(normalized, lockToken)
		}
	}()
	headers := make(http.Header)
	headers.Set("If", "("+lockToken+")")
	headers.Set("Content-Type", imageData.mimeType)
	response, requestErr := client.request(ctx, http.MethodPut, normalized, false, bytes.NewReader(imageData.data), headers)
	putAccepted := false
	if requestErr == nil {
		defer response.Body.Close()
		if statusErr := requireWebDAVStatus(response, "upload image", normalized, http.StatusOK, http.StatusCreated, http.StatusNoContent); statusErr != nil {
			drainWebDAVResponse(response.Body)
			requestErr = statusErr
		} else {
			putAccepted = true
			drainWebDAVResponse(response.Body)
		}
	}
	verified, representationState, verifyErr := client.readImageRepresentation(ctx, normalized, "verify uploaded image")
	if representationState == webDAVImageRepresentationValid && verifyErr == nil && verified.sha256 == imageData.sha256 && bytes.Equal(verified.data, imageData.data) {
		return WebDAVWriteResult{Path: normalized, Created: true, ConcurrencyMode: "dav-lock-create"}, nil
	}
	if !putAccepted && (representationState == webDAVImageRepresentationMissing || representationState == webDAVImageRepresentationLockNull) {
		// The PUT was not acknowledged and an authenticated GET confirms there
		// is no committed representation (either 404 or the server's explicit
		// zero-byte lock-null placeholder). It is now safe to remove that
		// placeholder before unlocking it.
		deleteLockNull = true
	}
	if requestErr != nil {
		return WebDAVWriteResult{}, requestErr
	}
	if verifyErr != nil {
		return WebDAVWriteResult{}, verifyErr
	}
	return WebDAVWriteResult{}, &WebDAVError{Kind: WebDAVErrorConflict, Operation: "verify uploaded image", Path: normalized, Err: errors.New("remote image bytes differ after upload")}
}

func fetchPublicImage(ctx context.Context, source string) (validatedImage, error) {
	parsed, err := validatePublicImageURL(source)
	if err != nil {
		return validatedImage{}, err
	}
	transport := &http.Transport{
		Proxy:                  nil,
		DialContext:            dialPublicImageAddress,
		ForceAttemptHTTP2:      true,
		MaxIdleConns:           2,
		MaxIdleConnsPerHost:    2,
		IdleConnTimeout:        15 * time.Second,
		TLSHandshakeTimeout:    10 * time.Second,
		ResponseHeaderTimeout:  10 * time.Second,
		MaxResponseHeaderBytes: 64 << 10,
		TLSClientConfig:        &tls.Config{MinVersion: tls.VersionTLS12},
	}
	defer transport.CloseIdleConnections()
	origin := *parsed
	client := &http.Client{
		Transport: transport,
		Timeout:   publicImageTimeout,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 4 {
				return errors.New("公开图片重定向次数过多")
			}
			candidate, validationErr := validatePublicImageURL(request.URL.String())
			if validationErr != nil || !sameWebDAVOrigin(&origin, candidate) {
				return errors.New("公开图片重定向离开了原始 HTTPS 站点")
			}
			return nil
		},
	}
	if ctx == nil {
		ctx = context.Background()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return validatedImage{}, errors.New("公开图片地址无效")
	}
	request.Header.Set("Accept", "image/png, image/jpeg, image/gif, image/webp")
	request.Header.Set("User-Agent", "InkMark/"+appVersion)
	response, err := client.Do(request)
	if err != nil {
		return validatedImage{}, fmt.Errorf("下载公开图片失败: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return validatedImage{}, fmt.Errorf("下载公开图片失败: HTTP %d", response.StatusCode)
	}
	if response.ContentLength > maxImageAssetSize {
		return validatedImage{}, imageSizeError()
	}
	imageData, err := readValidatedImage(response.Body, safeImageName(path.Base(parsed.Path), "public-image"), "")
	if err != nil {
		return validatedImage{}, err
	}
	return imageData, nil
}

func validatePublicImageURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !parsed.IsAbs() || !strings.EqualFold(parsed.Scheme, "https") || parsed.Host == "" {
		return nil, errors.New("公开图片必须使用有效的 HTTPS 地址")
	}
	if parsed.User != nil || parsed.Opaque != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return nil, errors.New("公开图片地址不能包含凭据、不明确查询或片段")
	}
	if port := parsed.Port(); port != "" && port != "443" {
		return nil, errors.New("公开图片 HTTPS 地址仅允许标准 443 端口")
	}
	hostname := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if hostname == "" || strings.ContainsAny(hostname, "\r\n\t ") || hostname == "localhost" || strings.HasSuffix(hostname, ".localhost") || strings.HasSuffix(hostname, ".local") || strings.HasSuffix(hostname, ".internal") {
		return nil, errors.New("公开图片地址不能指向本机或内部主机")
	}
	parsed.Scheme = "https"
	if address := net.ParseIP(hostname); address != nil && strings.Contains(hostname, ":") {
		parsed.Host = "[" + hostname + "]"
	} else {
		parsed.Host = hostname
	}
	return parsed, nil
}

func dialPublicImageAddress(ctx context.Context, network string, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || port != "443" {
		return nil, errors.New("公开图片连接地址无效")
	}
	addresses := make([]net.IPAddr, 0, 4)
	if parsed := net.ParseIP(host); parsed != nil {
		addresses = append(addresses, net.IPAddr{IP: parsed})
	} else {
		resolved, resolveErr := net.DefaultResolver.LookupIPAddr(ctx, host)
		if resolveErr != nil {
			return nil, fmt.Errorf("解析公开图片主机失败: %w", resolveErr)
		}
		addresses = append(addresses, resolved...)
	}
	if len(addresses) == 0 {
		return nil, errors.New("公开图片主机没有可用地址")
	}
	for _, address := range addresses {
		if !isPublicImageIP(address.IP) {
			return nil, errors.New("公开图片主机解析到了非公网地址")
		}
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 15 * time.Second}
	var lastErr error
	for _, resolved := range addresses {
		connection, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(resolved.IP.String(), port))
		if dialErr == nil {
			return connection, nil
		}
		lastErr = dialErr
	}
	return nil, lastErr
}

func isPublicImageIP(ip net.IP) bool {
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	address = address.Unmap()
	if !address.IsValid() || !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsMulticast() || address.IsUnspecified() {
		return false
	}
	for _, blocked := range publicImageBlockedNetworks {
		if blocked.Contains(address) {
			return false
		}
	}
	return true
}

var publicImageBlockedNetworks = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.31.196.0/24"),
	netip.MustParsePrefix("192.52.193.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("192.175.48.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	// IPv6 special-purpose ranges which netip correctly describes as global
	// unicast, but which are not safe public-image destinations. In particular,
	// NAT64 and 6to4 addresses can encode a private IPv4 destination.
	netip.MustParsePrefix("::/96"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("100:0:0:1::/64"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("3fff::/20"),
	netip.MustParsePrefix("5f00::/16"),
	netip.MustParsePrefix("fec0::/10"),
	netip.MustParsePrefix("2620:4f:8000::/48"),
}
