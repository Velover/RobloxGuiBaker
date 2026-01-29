package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/image/draw"
)

// Data structures matching the Luau JSON output
type Vector2 struct {
	X float64 `json:"X"`
	Y float64 `json:"Y"`
}

type Vector3 struct {
	X float64 `json:"X"`
	Y float64 `json:"Y"`
	Z float64 `json:"Z"`
}

type UDim struct {
	Scale  float64 `json:"Scale"`
	Offset float64 `json:"Offset"`
}

type UDim2 struct {
	X UDim `json:"X"`
	Y UDim `json:"Y"`
}

type Color3 struct {
	R float64 `json:"R"`
	G float64 `json:"G"`
	B float64 `json:"B"`
}

type NumberKeypoint struct {
	Time     float64 `json:"Time"`
	Value    float64 `json:"Value"`
	Envelope float64 `json:"Envelope"`
}

type ColorKeypoint struct {
	Time  float64 `json:"Time"`
	Value Color3  `json:"Value"`
}

type NumberSequence struct {
	Keypoints []NumberKeypoint `json:"Keypoints"`
}

type ColorSequence struct {
	Keypoints []ColorKeypoint `json:"Keypoints"`
}

type UIGradient struct {
	Enabled      bool           `json:"Enabled"`
	Rotation     float64        `json:"Rotation"`
	Transparency NumberSequence `json:"Transparency"`
	Color        ColorSequence  `json:"Color"`
	Offset       Vector2        `json:"Offset"`
}

type UIStroke struct {
	Enabled         bool        `json:"Enabled"`
	Color           Color3      `json:"Color"`
	Thickness       float64     `json:"Thickness"`
	Transparency    float64     `json:"Transparency"`
	ApplyStrokeMode string      `json:"ApplyStrokeMode"`
	UIGradient      *UIGradient `json:"UIGradient,omitempty"`
}

type UICorner struct {
	CornerRadius UDim `json:"CornerRadius"`
}

type GuiElement struct {
	Name                   string      `json:"Name"`
	ClassName              string      `json:"ClassName"`
	AbsoluteSize           Vector2     `json:"AbsoluteSize"`
	AbsolutePosition       Vector2     `json:"AbsolutePosition"`
	Visible                bool        `json:"Visible"`
	ZIndex                 int         `json:"ZIndex"`
	Size                   *UDim2      `json:"Size,omitempty"`
	Position               *UDim2      `json:"Position,omitempty"`
	AnchorPoint            *Vector2    `json:"AnchorPoint,omitempty"`
	Rotation               *float64    `json:"Rotation,omitempty"`
	BackgroundColor3       *Color3     `json:"BackgroundColor3,omitempty"`
	BackgroundTransparency *float64    `json:"BackgroundTransparency,omitempty"`
	BorderSizePixel        *int        `json:"BorderSizePixel,omitempty"`
	BorderColor3           *Color3     `json:"BorderColor3,omitempty"`
	LayoutOrder            *int        `json:"LayoutOrder,omitempty"`
	UIGradient             *UIGradient `json:"UIGradient,omitempty"`
	UIStroke               *UIStroke   `json:"UIStroke,omitempty"`
	UICorner               *UICorner   `json:"UICorner,omitempty"`
	Image                  *string     `json:"Image,omitempty"`
	ImageColor3            *Color3     `json:"ImageColor3,omitempty"`
	ImageTransparency      *float64    `json:"ImageTransparency,omitempty"`
	ImageRectOffset        *Vector2    `json:"ImageRectOffset,omitempty"`
	ImageRectSize          *Vector2    `json:"ImageRectSize,omitempty"`
	ScaleType              *string     `json:"ScaleType,omitempty"`
	CanvasSize             *UDim2      `json:"CanvasSize,omitempty"`
	ScrollBarThickness     *int        `json:"ScrollBarThickness,omitempty"`
}

// BoundingBox represents the axis-aligned bounding box
type BoundingBox struct {
	MinX float64
	MinY float64
	MaxX float64
	MaxY float64
}

func (bb BoundingBox) Width() float64  { return bb.MaxX - bb.MinX }
func (bb BoundingBox) Height() float64 { return bb.MaxY - bb.MinY }

// RotatePoint rotates a point around a center point by the given angle (in degrees)
func RotatePoint(px, py, cx, cy, angleDeg float64) (float64, float64) {
	angleRad := angleDeg * math.Pi / 180.0
	px -= cx
	py -= cy
	cosA := math.Cos(angleRad)
	sinA := math.Sin(angleRad)
	newX := px*cosA - py*sinA
	newY := px*sinA + py*cosA
	return newX + cx, newY + cy
}

// InterpolateColor interpolates between color keypoints
func InterpolateColor(colorSeq ColorSequence, t float64) Color3 {
	if len(colorSeq.Keypoints) == 0 {
		return Color3{R: 1, G: 1, B: 1}
	}
	if len(colorSeq.Keypoints) == 1 {
		return colorSeq.Keypoints[0].Value
	}

	// Find surrounding keypoints
	for i := 0; i < len(colorSeq.Keypoints)-1; i++ {
		k1 := colorSeq.Keypoints[i]
		k2 := colorSeq.Keypoints[i+1]
		if t >= k1.Time && t <= k2.Time {
			alpha := (t - k1.Time) / (k2.Time - k1.Time)
			return Color3{
				R: k1.Value.R + alpha*(k2.Value.R-k1.Value.R),
				G: k1.Value.G + alpha*(k2.Value.G-k1.Value.G),
				B: k1.Value.B + alpha*(k2.Value.B-k1.Value.B),
			}
		}
	}

	if t <= colorSeq.Keypoints[0].Time {
		return colorSeq.Keypoints[0].Value
	}
	return colorSeq.Keypoints[len(colorSeq.Keypoints)-1].Value
}

// InterpolateTransparency interpolates between transparency keypoints
func InterpolateTransparency(transSeq NumberSequence, t float64) float64 {
	if len(transSeq.Keypoints) == 0 {
		return 0
	}
	if len(transSeq.Keypoints) == 1 {
		return transSeq.Keypoints[0].Value
	}

	for i := 0; i < len(transSeq.Keypoints)-1; i++ {
		k1 := transSeq.Keypoints[i]
		k2 := transSeq.Keypoints[i+1]
		if t >= k1.Time && t <= k2.Time {
			alpha := (t - k1.Time) / (k2.Time - k1.Time)
			return k1.Value + alpha*(k2.Value-k1.Value)
		}
	}

	if t <= transSeq.Keypoints[0].Time {
		return transSeq.Keypoints[0].Value
	}
	return transSeq.Keypoints[len(transSeq.Keypoints)-1].Value
}

// ApplyGradientToPixel applies gradient to a pixel based on position
func ApplyGradientToPixel(baseColor Color3, baseAlpha float64, gradient *UIGradient, x, y, w, h float64) color.RGBA {
	if gradient == nil || !gradient.Enabled {
		return Color3ToRGBA(baseColor, baseAlpha)
	}

	// Calculate gradient position (0 to 1) considering rotation
	angleRad := gradient.Rotation * math.Pi / 180.0

	// Normalize coordinates to center
	nx := (x / w) - 0.5
	ny := (y / h) - 0.5

	// Rotate coordinates
	rotX := nx*math.Cos(angleRad) + ny*math.Sin(angleRad)

	// Map to 0-1 range
	t := rotX + 0.5
	t = math.Max(0, math.Min(1, t))

	// Interpolate gradient color and transparency
	gradColor := InterpolateColor(gradient.Color, t)
	gradTrans := InterpolateTransparency(gradient.Transparency, t)

	// Multiply gradient color with base color
	finalColor := Color3{
		R: baseColor.R * gradColor.R,
		G: baseColor.G * gradColor.G,
		B: baseColor.B * gradColor.B,
	}

	// Combine transparencies
	finalAlpha := baseAlpha + gradTrans - (baseAlpha * gradTrans)

	return Color3ToRGBA(finalColor, finalAlpha)
}

// CalculateBoundingBox calculates the axis-aligned bounding box of a rotated rectangle
func CalculateBoundingBox(element GuiElement) BoundingBox {
	absPos := element.AbsolutePosition
	absSize := element.AbsoluteSize
	rotation := 0.0
	if element.Rotation != nil {
		rotation = *element.Rotation
	}

	// Add stroke thickness to bounds if present
	strokeThickness := 0.0
	if element.UIStroke != nil && element.UIStroke.Enabled {
		strokeThickness = element.UIStroke.Thickness
	}

	cx := absPos.X + absSize.X*0.5
	cy := absPos.Y + absSize.Y*0.5

	corners := []Vector2{
		{X: absPos.X - strokeThickness, Y: absPos.Y - strokeThickness},
		{X: absPos.X + absSize.X + strokeThickness, Y: absPos.Y - strokeThickness},
		{X: absPos.X + absSize.X + strokeThickness, Y: absPos.Y + absSize.Y + strokeThickness},
		{X: absPos.X - strokeThickness, Y: absPos.Y + absSize.Y + strokeThickness},
	}

	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)

	for _, corner := range corners {
		rx, ry := RotatePoint(corner.X, corner.Y, cx, cy, rotation)
		minX = math.Min(minX, rx)
		minY = math.Min(minY, ry)
		maxX = math.Max(maxX, rx)
		maxY = math.Max(maxY, ry)
	}

	return BoundingBox{MinX: minX, MinY: minY, MaxX: maxX, MaxY: maxY}
}

// CalculateOverallBounds gets the bounding box of all visible elements
func CalculateOverallBounds(elements []GuiElement) BoundingBox {
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)

	for _, elem := range elements {
		if !elem.Visible {
			continue
		}
		bbox := CalculateBoundingBox(elem)
		minX = math.Min(minX, bbox.MinX)
		minY = math.Min(minY, bbox.MinY)
		maxX = math.Max(maxX, bbox.MaxX)
		maxY = math.Max(maxY, bbox.MaxY)
	}

	return BoundingBox{MinX: minX, MinY: minY, MaxX: maxX, MaxY: maxY}
}

// ExtractAssetID extracts the numeric asset ID from rbxassetid:// URL
func ExtractAssetID(imageURL string) string {
	re := regexp.MustCompile(`rbxassetid://(\d+)`)
	matches := re.FindStringSubmatch(imageURL)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// DownloadAsset downloads an asset from Roblox asset delivery
func DownloadAsset(assetID, outputDir string) (string, error) {
	url := fmt.Sprintf("https://assetdelivery.roblox.com/v1/asset?id=%s", assetID)

	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to download asset %s: %w", assetID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download asset %s: status %d", assetID, resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	ext := ".png"

	if strings.Contains(contentType, "image/png") {
		ext = ".png"
	} else if strings.Contains(contentType, "image/jpeg") || strings.Contains(contentType, "image/jpg") {
		ext = ".jpg"
	} else {
		fmt.Printf("Warning: Asset %s has content type %s\n", assetID, contentType)
	}

	os.MkdirAll(outputDir, 0755)
	filename := filepath.Join(outputDir, assetID+ext)
	outFile, err := os.Create(filename)
	if err != nil {
		return "", fmt.Errorf("failed to create file %s: %w", filename, err)
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to write file %s: %w", filename, err)
	}

	return filename, nil
}

// Color3ToRGBA converts Roblox Color3 to Go color.RGBA
func Color3ToRGBA(c Color3, alpha float64) color.RGBA {
	return color.RGBA{
		R: uint8(c.R * 255),
		G: uint8(c.G * 255),
		B: uint8(c.B * 255),
		A: uint8((1 - alpha) * 255),
	}
}

// ScaleImage scales an image to fit the target dimensions
func ScaleImage(src image.Image, width, height int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.BiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	return dst
}

// CreateElementImage creates an unrotated image of the element with stroke
func CreateElementImage(element GuiElement, assetCache map[string]image.Image) *image.RGBA {
	w := int(element.AbsoluteSize.X)
	h := int(element.AbsoluteSize.Y)

	strokeThickness := 0.0
	if element.UIStroke != nil && element.UIStroke.Enabled {
		strokeThickness = element.UIStroke.Thickness
	}

	// Expand canvas for stroke
	totalW := int(float64(w) + strokeThickness*2)
	totalH := int(float64(h) + strokeThickness*2)

	img := image.NewRGBA(image.Rect(0, 0, totalW, totalH))

	// Draw stroke first (under the content)
	if element.UIStroke != nil && element.UIStroke.Enabled {
		stroke := element.UIStroke
		strokeColor := stroke.Color
		strokeTrans := stroke.Transparency

		for py := 0; py < totalH; py++ {
			for px := 0; px < totalW; px++ {
				// Calculate distance from edge of content rectangle
				contentX := float64(px) - strokeThickness
				contentY := float64(py) - strokeThickness

				// Check if pixel is in stroke region
				inStroke := false
				if contentX < 0 || contentX >= float64(w) || contentY < 0 || contentY >= float64(h) {
					// Outside content area - check if within stroke thickness
					distX := math.Max(0, math.Max(-contentX, contentX-float64(w)))
					distY := math.Max(0, math.Max(-contentY, contentY-float64(h)))
					dist := math.Max(distX, distY)
					if dist <= strokeThickness {
						inStroke = true
					}
				}

				if inStroke {
					// Apply gradient if present
					gradientColor := ApplyGradientToPixel(
						strokeColor,
						strokeTrans,
						stroke.UIGradient,
						float64(px),
						float64(py),
						float64(totalW),
						float64(totalH),
					)
					img.Set(px, py, gradientColor)
				}
			}
		}
	}

	// Draw background if present
	offsetX := int(strokeThickness)
	offsetY := int(strokeThickness)

	if element.BackgroundColor3 != nil && element.BackgroundTransparency != nil {
		bgColor := *element.BackgroundColor3
		bgTrans := *element.BackgroundTransparency

		for py := 0; py < h; py++ {
			for px := 0; px < w; px++ {
				pixelColor := ApplyGradientToPixel(
					bgColor,
					bgTrans,
					element.UIGradient,
					float64(px),
					float64(py),
					float64(w),
					float64(h),
				)
				img.Set(px+offsetX, py+offsetY, pixelColor)
			}
		}
	}

	// Draw image if present
	if element.Image != nil && *element.Image != "" {
		assetID := ExtractAssetID(*element.Image)
		if assetID != "" {
			if srcImg, ok := assetCache[assetID]; ok {
				scaledImg := ScaleImage(srcImg, w, h)

				imgColor := Color3{R: 1, G: 1, B: 1}
				if element.ImageColor3 != nil {
					imgColor = *element.ImageColor3
				}

				for py := 0; py < h; py++ {
					for px := 0; px < w; px++ {
						r, g, b, a := scaledImg.At(px, py).RGBA()
						if a > 0 {
							// Apply image color tint and gradient
							baseColor := Color3{
								R: imgColor.R * float64(r) / 65535.0,
								G: imgColor.G * float64(g) / 65535.0,
								B: imgColor.B * float64(b) / 65535.0,
							}

							baseAlpha := float64(a) / 65535.0
							if element.ImageTransparency != nil {
								baseAlpha *= (1 - *element.ImageTransparency)
							}

							pixelColor := ApplyGradientToPixel(
								baseColor,
								1-baseAlpha,
								element.UIGradient,
								float64(px),
								float64(py),
								float64(w),
								float64(h),
							)

							// Blend with existing pixel
							existing := img.At(px+offsetX, py+offsetY)
							er, eg, eb, ea := existing.RGBA()
							pr, pg, pb, pa := pixelColor.RGBA()

							alpha := float64(pa) / 65535.0
							invAlpha := 1 - alpha

							img.Set(px+offsetX, py+offsetY, color.RGBA{
								R: uint8((float64(er>>8)*invAlpha + float64(pr>>8)*alpha)),
								G: uint8((float64(eg>>8)*invAlpha + float64(pg>>8)*alpha)),
								B: uint8((float64(eb>>8)*invAlpha + float64(pb>>8)*alpha)),
								A: uint8(math.Min(255, float64(ea>>8)+float64(pa>>8))),
							})
						}
					}
				}
			}
		}
	}

	return img
}

// RotateImage rotates an image around its center by the given angle (in degrees)
func RotateImage(src *image.RGBA, angleDeg float64) *image.RGBA {
	if angleDeg == 0 {
		return src
	}

	angleRad := -angleDeg * math.Pi / 180.0
	srcBounds := src.Bounds()
	srcW := float64(srcBounds.Dx())
	srcH := float64(srcBounds.Dy())

	cosA := math.Abs(math.Cos(angleRad))
	sinA := math.Abs(math.Sin(angleRad))
	dstW := int(math.Ceil(srcW*cosA + srcH*sinA))
	dstH := int(math.Ceil(srcW*sinA + srcH*cosA))

	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))

	srcCx := srcW / 2
	srcCy := srcH / 2
	dstCx := float64(dstW) / 2
	dstCy := float64(dstH) / 2

	for dstY := 0; dstY < dstH; dstY++ {
		for dstX := 0; dstX < dstW; dstX++ {
			x := float64(dstX) - dstCx
			y := float64(dstY) - dstCy

			srcX := x*math.Cos(angleRad) - y*math.Sin(angleRad) + srcCx
			srcY := x*math.Sin(angleRad) + y*math.Cos(angleRad) + srcCy

			if srcX >= 0 && srcX < srcW-1 && srcY >= 0 && srcY < srcH-1 {
				x0 := int(srcX)
				y0 := int(srcY)
				x1 := x0 + 1
				y1 := y0 + 1

				fx := srcX - float64(x0)
				fy := srcY - float64(y0)

				c00 := src.At(x0, y0)
				c10 := src.At(x1, y0)
				c01 := src.At(x0, y1)
				c11 := src.At(x1, y1)

				r00, g00, b00, a00 := c00.RGBA()
				r10, g10, b10, a10 := c10.RGBA()
				r01, g01, b01, a01 := c01.RGBA()
				r11, g11, b11, a11 := c11.RGBA()

				r := (1-fx)*(1-fy)*float64(r00) + fx*(1-fy)*float64(r10) + (1-fx)*fy*float64(r01) + fx*fy*float64(r11)
				g := (1-fx)*(1-fy)*float64(g00) + fx*(1-fy)*float64(g10) + (1-fx)*fy*float64(g01) + fx*fy*float64(g11)
				b := (1-fx)*(1-fy)*float64(b00) + fx*(1-fy)*float64(b10) + (1-fx)*fy*float64(b01) + fx*fy*float64(b11)
				a := (1-fx)*(1-fy)*float64(a00) + fx*(1-fy)*float64(a10) + (1-fx)*fy*float64(a01) + fx*fy*float64(a11)

				dst.Set(dstX, dstY, color.RGBA{
					R: uint8(r / 257),
					G: uint8(g / 257),
					B: uint8(b / 257),
					A: uint8(a / 257),
				})
			}
		}
	}

	return dst
}

// DrawElement renders a GUI element onto the canvas
func DrawElement(canvas *image.RGBA, element GuiElement, bounds BoundingBox, assetCache map[string]image.Image) {
	if !element.Visible {
		return
	}

	rotation := 0.0
	if element.Rotation != nil {
		rotation = *element.Rotation
	}

	elemImg := CreateElementImage(element, assetCache)

	if rotation != 0 {
		elemImg = RotateImage(elemImg, rotation)
	}

	absPos := element.AbsolutePosition
	absSize := element.AbsoluteSize
	centerX := absPos.X + absSize.X*0.5 - bounds.MinX
	centerY := absPos.Y + absSize.Y*0.5 - bounds.MinY

	rotW := float64(elemImg.Bounds().Dx())
	rotH := float64(elemImg.Bounds().Dy())
	dstX := int(centerX - rotW/2)
	dstY := int(centerY - rotH/2)

	dstRect := image.Rect(dstX, dstY, dstX+int(rotW), dstY+int(rotH))
	draw.Draw(canvas, dstRect, elemImg, image.Point{}, draw.Over)
}

// BakeToImage renders all GUI elements to a PNG file
func BakeToImage(elements []GuiElement, outputPath string, assetDir string) error {
	sort.SliceStable(elements, func(i, j int) bool {
		return elements[i].ZIndex < elements[j].ZIndex
	})

	bounds := CalculateOverallBounds(elements)
	width := int(math.Ceil(bounds.Width()))
	height := int(math.Ceil(bounds.Height()))

	fmt.Printf("Canvas size: %dx%d\n", width, height)

	scale := 1.0
	maxDim := math.Max(float64(width), float64(height))
	if maxDim > 1024 {
		scale = 1024 / maxDim
		width = int(float64(width) * scale)
		height = int(float64(height) * scale)
		fmt.Printf("Scaled down to: %dx%d (scale: %.3f)\n", width, height, scale)
	}

	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{color.Transparent}, image.Point{}, draw.Src)

	assetCache := make(map[string]image.Image)
	for _, elem := range elements {
		if elem.Image != nil && *elem.Image != "" {
			assetID := ExtractAssetID(*elem.Image)
			if assetID != "" && assetCache[assetID] == nil {
				assetPath := filepath.Join(assetDir, assetID+".png")
				if imgFile, err := os.Open(assetPath); err == nil {
					if img, err := png.Decode(imgFile); err == nil {
						assetCache[assetID] = img
						fmt.Printf("✓ Loaded asset: %s\n", assetID)
					}
					imgFile.Close()
				}
			}
		}
	}

	if scale != 1.0 {
		for i := range elements {
			elements[i].AbsolutePosition.X = (elements[i].AbsolutePosition.X - bounds.MinX) * scale
			elements[i].AbsolutePosition.Y = (elements[i].AbsolutePosition.Y - bounds.MinY) * scale
			elements[i].AbsoluteSize.X *= scale
			elements[i].AbsoluteSize.Y *= scale
			if elements[i].UIStroke != nil {
				elements[i].UIStroke.Thickness *= scale
			}
		}
		bounds.MinX = 0
		bounds.MinY = 0
	}

	for _, elem := range elements {
		DrawElement(canvas, elem, bounds, assetCache)
	}

	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	err = png.Encode(outFile, canvas)
	if err != nil {
		return fmt.Errorf("failed to encode PNG: %w", err)
	}

	fmt.Printf("\n✓ Baked image saved to: %s\n", outputPath)
	return nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <json_file> [output_dir]")
		os.Exit(1)
	}

	jsonFile := os.Args[1]
	outputDir := "assets"

	if len(os.Args) >= 3 {
		outputDir = os.Args[2]
	}

	data, err := os.ReadFile(jsonFile)
	if err != nil {
		fmt.Printf("Error reading JSON file: %v\n", err)
		os.Exit(1)
	}

	var elements []GuiElement
	err = json.Unmarshal(data, &elements)
	if err != nil {
		fmt.Printf("Error parsing JSON: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Parsed %d GUI elements\n\n", len(elements))

	downloadedAssets := make(map[string]bool)
	for _, element := range elements {
		if element.Image != nil && *element.Image != "" {
			assetID := ExtractAssetID(*element.Image)
			if assetID != "" && !downloadedAssets[assetID] {
				fmt.Printf("Downloading image asset: %s\n", assetID)
				_, err := DownloadAsset(assetID, outputDir)
				if err != nil {
					fmt.Printf("Error: %v\n", err)
				} else {
					downloadedAssets[assetID] = true
				}
			}
		}
	}

	fmt.Printf("\n✓ Downloaded %d unique assets\n\n", len(downloadedAssets))

	outputPath := "baked_gui.png"
	err = BakeToImage(elements, outputPath, outputDir)
	if err != nil {
		fmt.Printf("Error baking image: %v\n", err)
		os.Exit(1)
	}
}
