package main

import (
	"encoding/json"
	"flag"
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

	"github.com/joho/godotenv"
	"golang.org/x/image/draw"
)

// Global variable for Roblox cookie
var robloxCookie string

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
	Id                     string      `json:"Id"`
	ParentId               string      `json:"ParentId"`
	Name                   string      `json:"Name"`
	ClassName              string      `json:"ClassName"`
	AbsoluteSize           Vector2     `json:"AbsoluteSize"`
	AbsolutePosition       Vector2     `json:"AbsolutePosition"`
	AbsoluteRotation       *float64    `json:"AbsoluteRotation,omitempty"`
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
	GroupTransparency      *float64    `json:"GroupTransparency,omitempty"`
	GroupColor3            *Color3     `json:"GroupColor3,omitempty"`
	Image                  *string     `json:"Image,omitempty"`
	ImageColor3            *Color3     `json:"ImageColor3,omitempty"`
	ImageTransparency      *float64    `json:"ImageTransparency,omitempty"`
	ImageRectOffset        *Vector2    `json:"ImageRectOffset,omitempty"`
	ImageRectSize          *Vector2    `json:"ImageRectSize,omitempty"`
	ScaleType              *string     `json:"ScaleType,omitempty"`
	CanvasSize             *UDim2      `json:"CanvasSize,omitempty"`
	ScrollBarThickness     *int        `json:"ScrollBarThickness,omitempty"`

	// Runtime fields
	Children []*GuiElement `json:"-"`
}

// ElementHierarchy manages parent-child relationships
type ElementHierarchy struct {
	Elements map[string]*GuiElement
	Roots    []*GuiElement
}

// BuildHierarchy constructs the element tree
func BuildHierarchy(elements []GuiElement) *ElementHierarchy {
	hierarchy := &ElementHierarchy{
		Elements: make(map[string]*GuiElement),
		Roots:    make([]*GuiElement, 0),
	}

	// First pass: index all elements
	for i := range elements {
		elem := &elements[i]
		hierarchy.Elements[elem.Id] = elem
	}

	// Second pass: build parent-child relationships
	for _, elem := range hierarchy.Elements {
		if elem.ParentId == "" {
			hierarchy.Roots = append(hierarchy.Roots, elem)
		} else if parent, ok := hierarchy.Elements[elem.ParentId]; ok {
			parent.Children = append(parent.Children, elem)
		}
	}

	// Sort children by ZIndex within each parent
	for _, elem := range hierarchy.Elements {
		sort.SliceStable(elem.Children, func(i, j int) bool {
			return elem.Children[i].ZIndex < elem.Children[j].ZIndex
		})
	}

	// Sort roots by ZIndex
	sort.SliceStable(hierarchy.Roots, func(i, j int) bool {
		return hierarchy.Roots[i].ZIndex < hierarchy.Roots[j].ZIndex
	})

	return hierarchy
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
func ApplyGradientToPixel(baseColor Color3, baseTransparency float64, gradient *UIGradient, x, y, w, h float64) color.NRGBA {
	if gradient == nil || !gradient.Enabled {
		return Color3ToNRGBA(baseColor, baseTransparency)
	}

	angleRad := gradient.Rotation * math.Pi / 180.0
	nx := (x / w) - 0.5
	ny := (y / h) - 0.5
	rotX := nx*math.Cos(angleRad) + ny*math.Sin(angleRad)
	t := math.Max(0, math.Min(1, rotX+0.5))

	gradColor := InterpolateColor(gradient.Color, t)
	gradTrans := InterpolateTransparency(gradient.Transparency, t)

	finalColor := Color3{
		R: baseColor.R * gradColor.R,
		G: baseColor.G * gradColor.G,
		B: baseColor.B * gradColor.B,
	}

	baseAlpha := 1 - baseTransparency
	gradAlpha := 1 - gradTrans
	finalAlpha := baseAlpha * gradAlpha

	return color.NRGBA{
		R: uint8(math.Round(finalColor.R * 255)),
		G: uint8(math.Round(finalColor.G * 255)),
		B: uint8(math.Round(finalColor.B * 255)),
		A: uint8(math.Round(finalAlpha * 255)),
	}
}

// CalculateBoundingBox calculates the axis-aligned bounding box
func CalculateBoundingBox(element *GuiElement) BoundingBox {
	absPos := element.AbsolutePosition
	absSize := element.AbsoluteSize
	rotation := 0.0
	if element.AbsoluteRotation != nil {
		rotation = *element.AbsoluteRotation
	}

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

// CalculateOverallBounds recursively calculates bounds for hierarchy
func CalculateOverallBounds(hierarchy *ElementHierarchy) BoundingBox {
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)

	var processBounds func(*GuiElement)
	processBounds = func(elem *GuiElement) {
		if !elem.Visible {
			return
		}
		bbox := CalculateBoundingBox(elem)
		minX = math.Min(minX, bbox.MinX)
		minY = math.Min(minY, bbox.MinY)
		maxX = math.Max(maxX, bbox.MaxX)
		maxY = math.Max(maxY, bbox.MaxY)

		for _, child := range elem.Children {
			processBounds(child)
		}
	}

	for _, root := range hierarchy.Roots {
		processBounds(root)
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

	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request for asset %s: %w", assetID, err)
	}

	// Add Roblox cookie if available
	if robloxCookie != "" {
		req.AddCookie(&http.Cookie{
			Name:  ".ROBLOSECURITY",
			Value: robloxCookie,
		})
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download asset %s: %w", assetID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download asset %s: status %d (check ROBLOSECURITY in .env file)", assetID, resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	ext := ".png"

	if strings.Contains(contentType, "image/png") {
		ext = ".png"
	} else if strings.Contains(contentType, "image/jpeg") || strings.Contains(contentType, "image/jpg") {
		ext = ".jpg"
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

// Color3ToNRGBA converts Roblox Color3 to Go color.NRGBA
func Color3ToNRGBA(c Color3, transparency float64) color.NRGBA {
	return color.NRGBA{
		R: uint8(c.R * 255),
		G: uint8(c.G * 255),
		B: uint8(c.B * 255),
		A: uint8((1 - transparency) * 255),
	}
}

// ScaleImage scales an image to fit the target dimensions
func ScaleImage(src image.Image, width, height int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.BiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	return dst
}

func CalculateCornerRadius(cornerRadius UDim, width, height float64) float64 {
	minDim := math.Min(width, height)
	radius := cornerRadius.Scale*minDim + cornerRadius.Offset
	maxRadius := minDim / 2
	return math.Min(radius, maxRadius)
}

// IsInsideRoundedRect checks if a point is inside a rounded rectangle
func IsInsideRoundedRect(x, y, width, height, radius float64) bool {
	if radius <= 0 {
		return x >= 0 && x < width && y >= 0 && y < height
	}

	// FIXED: Changed <= to < for width/height checks
	if x < 0 || x >= width || y < 0 || y >= height {
		return false
	}

	inTopLeft := x < radius && y < radius
	inTopRight := x >= width-radius && y < radius
	inBottomLeft := x < radius && y >= height-radius
	inBottomRight := x >= width-radius && y >= height-radius

	if !inTopLeft && !inTopRight && !inBottomLeft && !inBottomRight {
		return true
	}

	if inTopLeft {
		dx := radius - x
		dy := radius - y
		return dx*dx+dy*dy <= radius*radius
	}

	if inTopRight {
		dx := x - (width - radius)
		dy := radius - y
		return dx*dx+dy*dy <= radius*radius
	}

	if inBottomLeft {
		dx := radius - x
		dy := y - (height - radius)
		return dx*dx+dy*dy <= radius*radius
	}

	if inBottomRight {
		dx := x - (width - radius)
		dy := y - (height - radius)
		return dx*dx+dy*dy <= radius*radius
	}

	return false
}

// CreateElementImage creates the element visual
func CreateElementImage(element *GuiElement, assetCache map[string]image.Image) *image.NRGBA {
	w := int(element.AbsoluteSize.X)
	h := int(element.AbsoluteSize.Y)

	strokeThickness := 0.0
	if element.UIStroke != nil && element.UIStroke.Enabled {
		strokeThickness = element.UIStroke.Thickness
	}

	cornerRadius := 0.0
	if element.UICorner != nil {
		cornerRadius = CalculateCornerRadius(element.UICorner.CornerRadius, float64(w), float64(h))
	}

	totalW := int(float64(w) + strokeThickness*2)
	totalH := int(float64(h) + strokeThickness*2)

	img := image.NewNRGBA(image.Rect(0, 0, totalW, totalH))

	// Draw stroke
	if element.UIStroke != nil && element.UIStroke.Enabled {
		stroke := element.UIStroke
		strokeColor := stroke.Color
		strokeTrans := stroke.Transparency
		outerRadius := cornerRadius + strokeThickness

		for py := 0; py < totalH; py++ {
			for px := 0; px < totalW; px++ {
				contentX := float64(px) - strokeThickness
				contentY := float64(py) - strokeThickness

				insideOuter := IsInsideRoundedRect(float64(px), float64(py), float64(totalW), float64(totalH), outerRadius)
				insideInner := IsInsideRoundedRect(contentX, contentY, float64(w), float64(h), cornerRadius)

				if insideOuter && !insideInner {
					gradientColor := ApplyGradientToPixel(
						strokeColor,
						strokeTrans,
						stroke.UIGradient,
						contentX,
						contentY,
						float64(w),
						float64(h),
					)
					img.SetNRGBA(px, py, gradientColor)
				}
			}
		}
	}

	offsetX := int(strokeThickness)
	offsetY := int(strokeThickness)

	// Draw background
	if element.BackgroundColor3 != nil && element.BackgroundTransparency != nil {
		bgColor := *element.BackgroundColor3
		bgTrans := *element.BackgroundTransparency

		for py := 0; py < h; py++ {
			for px := 0; px < w; px++ {
				if IsInsideRoundedRect(float64(px), float64(py), float64(w), float64(h), cornerRadius) {
					pixelColor := ApplyGradientToPixel(
						bgColor,
						bgTrans,
						element.UIGradient,
						float64(px),
						float64(py),
						float64(w),
						float64(h),
					)
					img.SetNRGBA(px+offsetX, py+offsetY, pixelColor)
				}
			}
		}
	}

	// Draw image
	if element.Image != nil && *element.Image != "" {
		assetID := ExtractAssetID(*element.Image)
		if assetID != "" {
			if srcImg, ok := assetCache[assetID]; ok {
				scaledImg := ScaleImage(srcImg, w, h)

				imgColor := Color3{R: 1, G: 1, B: 1}
				if element.ImageColor3 != nil {
					imgColor = *element.ImageColor3
				}

				imgTrans := 0.0
				if element.ImageTransparency != nil {
					imgTrans = *element.ImageTransparency
				}

				tempImg := image.NewNRGBA(image.Rect(0, 0, w, h))

				for py := 0; py < h; py++ {
					for px := 0; px < w; px++ {
						if IsInsideRoundedRect(float64(px), float64(py), float64(w), float64(h), cornerRadius) {
							r, g, b, a := scaledImg.At(px, py).RGBA()
							if a > 0 {
								srcAlpha := float64(a) / 65535.0

								var srcR, srcG, srcB float64
								if srcAlpha > 0 {
									srcR = float64(r) / 65535.0 / srcAlpha
									srcG = float64(g) / 65535.0 / srcAlpha
									srcB = float64(b) / 65535.0 / srcAlpha
								}

								finalAlpha := srcAlpha * (1 - imgTrans)

								baseColor := Color3{
									R: imgColor.R * srcR,
									G: imgColor.G * srcG,
									B: imgColor.B * srcB,
								}

								pixelColor := ApplyGradientToPixel(
									baseColor,
									1-finalAlpha,
									element.UIGradient,
									float64(px),
									float64(py),
									float64(w),
									float64(h),
								)

								tempImg.SetNRGBA(px, py, pixelColor)
							}
						}
					}
				}

				dstRect := image.Rect(offsetX, offsetY, offsetX+w, offsetY+h)
				draw.Draw(img, dstRect, tempImg, image.Point{}, draw.Over)
			}
		}
	}

	return img
}

// RotateImage rotates an image around its center
func RotateImage(src *image.NRGBA, angleDeg float64) *image.NRGBA {
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

	dst := image.NewNRGBA(image.Rect(0, 0, dstW, dstH))

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

				c00 := src.NRGBAAt(x0, y0)
				c10 := src.NRGBAAt(x1, y0)
				c01 := src.NRGBAAt(x0, y1)
				c11 := src.NRGBAAt(x1, y1)

				r := (1-fx)*(1-fy)*float64(c00.R) + fx*(1-fy)*float64(c10.R) + (1-fx)*fy*float64(c01.R) + fx*fy*float64(c11.R)
				g := (1-fx)*(1-fy)*float64(c00.G) + fx*(1-fy)*float64(c10.G) + (1-fx)*fy*float64(c01.G) + fx*fy*float64(c11.G)
				b := (1-fx)*(1-fy)*float64(c00.B) + fx*(1-fy)*float64(c10.B) + (1-fx)*fy*float64(c01.B) + fx*fy*float64(c11.B)
				a := (1-fx)*(1-fy)*float64(c00.A) + fx*(1-fy)*float64(c10.A) + (1-fx)*fy*float64(c01.A) + fx*fy*float64(c11.A)

				dst.SetNRGBA(dstX, dstY, color.NRGBA{
					R: uint8(r + 0.5),
					G: uint8(g + 0.5),
					B: uint8(b + 0.5),
					A: uint8(a + 0.5),
				})
			}
		}
	}

	return dst
}

// ClipToRect clips an image to a rectangle
func ClipToRect(src *image.NRGBA, clipX, clipY, clipW, clipH int) *image.NRGBA {
	bounds := src.Bounds()
	dst := image.NewNRGBA(bounds)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if x >= clipX && x < clipX+clipW && y >= clipY && y < clipY+clipH {
				dst.SetNRGBA(x, y, src.NRGBAAt(x, y))
			}
		}
	}

	return dst
}

// IsPointInRotatedRect checks if a pixel is inside a rotated rectangle
// px, py are canvas pixel coordinates (integers)
// The rectangle is defined in canvas space, centered at (centerX, centerY) with given width/height and rotation
func IsPointInRotatedRect(px, py int, centerX, centerY, width, height, rotationDeg float64) bool {
	if rotationDeg == 0 {
		// For non-rotated, check against axis-aligned bounds
		left := centerX - width*0.5
		top := centerY - height*0.5
		return float64(px) >= left && float64(px) < left+width &&
			float64(py) >= top && float64(py) < top+height
	}

	// For rotated rectangles, transform the pixel center to the rectangle's local space
	// Use pixel center for more accurate boundary determination
	pointX := float64(px) + 0.5
	pointY := float64(py) + 0.5

	// Translate to rectangle's center
	localX := pointX - centerX
	localY := pointY - centerY

	// Rotate back by negative rotation to get local coordinates
	angleRad := -rotationDeg * math.Pi / 180.0
	cosA := math.Cos(angleRad)
	sinA := math.Sin(angleRad)
	rotatedX := localX*cosA - localY*sinA
	rotatedY := localX*sinA + localY*cosA

	// Check if the point is inside the unrotated rectangle bounds
	halfW := width * 0.5
	halfH := height * 0.5

	return rotatedX >= -halfW && rotatedX < halfW && rotatedY >= -halfH && rotatedY < halfH
}

// ClipMask represents a clipping region with rotation support
type ClipMask struct {
	Rect     image.Rectangle
	Rotation float64
	CenterX  float64
	CenterY  float64
	Width    float64
	Height   float64
}

// IsInside checks if a canvas pixel is inside the clipping mask
func (cm *ClipMask) IsInside(x, y int) bool {
	// First check axis-aligned bounds for quick rejection
	if !image.Pt(x, y).In(cm.Rect) {
		return false
	}

	// If no rotation, we're done
	if cm.Rotation == 0 {
		return true
	}

	// Check rotated rectangle using the exact same logic as rendering
	return IsPointInRotatedRect(x, y, cm.CenterX, cm.CenterY, cm.Width, cm.Height, cm.Rotation)
}

// RenderElementRecursive renders element and its children
func RenderElementRecursive(canvas *image.NRGBA, element *GuiElement, bounds BoundingBox, assetCache map[string]image.Image, parentClip *ClipMask) {
	if !element.Visible {
		return
	}

	rotation := 0.0
	if element.AbsoluteRotation != nil {
		rotation = *element.AbsoluteRotation
	}

	elemImg := CreateElementImage(element, assetCache)

	// Apply rotation using AbsoluteRotation (already global)
	if rotation != 0 {
		elemImg = RotateImage(elemImg, rotation)
	}

	absPos := element.AbsolutePosition
	absSize := element.AbsoluteSize

	// Calculate center in canvas space
	centerX := absPos.X + absSize.X*0.5 - bounds.MinX
	centerY := absPos.Y + absSize.Y*0.5 - bounds.MinY

	rotW := float64(elemImg.Bounds().Dx())
	rotH := float64(elemImg.Bounds().Dy())
	dstX := int(centerX - rotW/2)
	dstY := int(centerY - rotH/2)

	// Draw element with clipping
	elemBounds := elemImg.Bounds()
	for srcY := elemBounds.Min.Y; srcY < elemBounds.Max.Y; srcY++ {
		for srcX := elemBounds.Min.X; srcX < elemBounds.Max.X; srcX++ {
			canvasX := dstX + srcX
			canvasY := dstY + srcY

			// Check canvas bounds
			if canvasX < 0 || canvasX >= canvas.Bounds().Dx() || canvasY < 0 || canvasY >= canvas.Bounds().Dy() {
				continue
			}

			// Check parent clipping
			if parentClip != nil && !parentClip.IsInside(canvasX, canvasY) {
				continue
			}

			// Alpha blend the pixel
			src := elemImg.NRGBAAt(srcX, srcY)
			if src.A > 0 {
				dst := canvas.NRGBAAt(canvasX, canvasY)

				// Alpha compositing: dst_out = src over dst
				srcAlpha := float64(src.A) / 255.0
				dstAlpha := float64(dst.A) / 255.0
				outAlpha := srcAlpha + dstAlpha*(1-srcAlpha)

				if outAlpha > 0 {
					outR := (float64(src.R)*srcAlpha + float64(dst.R)*dstAlpha*(1-srcAlpha)) / outAlpha
					outG := (float64(src.G)*srcAlpha + float64(dst.G)*dstAlpha*(1-srcAlpha)) / outAlpha
					outB := (float64(src.B)*srcAlpha + float64(dst.B)*dstAlpha*(1-srcAlpha)) / outAlpha

					canvas.SetNRGBA(canvasX, canvasY, color.NRGBA{
						R: uint8(outR),
						G: uint8(outG),
						B: uint8(outB),
						A: uint8(outAlpha * 255),
					})
				}
			}
		}
	}

	// Set up clipping for children if this is a CanvasGroup
	var childClip *ClipMask
	if element.ClassName == "CanvasGroup" {
		// Calculate bounding box for quick rejection
		bbox := CalculateBoundingBox(element)
		clipRect := image.Rect(
			int(math.Floor(bbox.MinX-bounds.MinX)),
			int(math.Floor(bbox.MinY-bounds.MinY)),
			int(math.Ceil(bbox.MaxX-bounds.MinX)),
			int(math.Ceil(bbox.MaxY-bounds.MinY)),
		)

		// Intersect with parent clip if exists
		if parentClip != nil {
			clipRect = clipRect.Intersect(parentClip.Rect)
		}

		// Ensure within canvas bounds
		clipRect = clipRect.Intersect(canvas.Bounds())

		// Use the SAME center calculation as drawing
		childClip = &ClipMask{
			Rect:     clipRect,
			Rotation: rotation,
			CenterX:  centerX, // Use the exact same center as when drawing
			CenterY:  centerY,
			Width:    absSize.X, // Use original size, not rotated size
			Height:   absSize.Y,
		}
	} else {
		childClip = parentClip
	}

	// Render children with their absolute positions and rotations
	for _, child := range element.Children {
		RenderElementRecursive(canvas, child, bounds, assetCache, childClip)
	}
}

// Update BakeToImage signature
func BakeToImage(hierarchy *ElementHierarchy, outputPath string, assetCache map[string]image.Image) (*image.NRGBA, error) {
	bounds := CalculateOverallBounds(hierarchy)
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

		// Scale all elements
		var scaleElement func(*GuiElement)
		scaleElement = func(elem *GuiElement) {
			elem.AbsolutePosition.X = (elem.AbsolutePosition.X - bounds.MinX) * scale
			elem.AbsolutePosition.Y = (elem.AbsolutePosition.Y - bounds.MinY) * scale
			elem.AbsoluteSize.X *= scale
			elem.AbsoluteSize.Y *= scale
			if elem.UIStroke != nil {
				elem.UIStroke.Thickness *= scale
			}
			if elem.UICorner != nil {
				elem.UICorner.CornerRadius.Offset *= scale
			}
			for _, child := range elem.Children {
				scaleElement(child)
			}
		}
		for _, root := range hierarchy.Roots {
			scaleElement(root)
		}
		bounds.MinX = 0
		bounds.MinY = 0
	}

	canvas := image.NewNRGBA(image.Rect(0, 0, width, height))

	// Render all root elements and their children
	for _, root := range hierarchy.Roots {
		RenderElementRecursive(canvas, root, bounds, assetCache, nil)
	}

	outFile, err := os.Create(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	err = png.Encode(outFile, canvas)
	if err != nil {
		return nil, fmt.Errorf("failed to encode PNG: %w", err)
	}

	fmt.Printf("\n✓ Baked image saved to: %s\n", outputPath)
	return canvas, nil
}

func SaveChannelDebugImages(img *image.NRGBA, baseName string) error {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Create separate images for each channel
	rImg := image.NewGray(bounds)
	gImg := image.NewGray(bounds)
	bImg := image.NewGray(bounds)
	aImg := image.NewGray(bounds)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			c := img.NRGBAAt(x, y)
			rImg.SetGray(x, y, color.Gray{Y: c.R})
			gImg.SetGray(x, y, color.Gray{Y: c.G})
			bImg.SetGray(x, y, color.Gray{Y: c.B})
			aImg.SetGray(x, y, color.Gray{Y: c.A})
		}
	}

	// Save each channel
	channels := map[string]*image.Gray{
		"_channel_R.png": rImg,
		"_channel_G.png": gImg,
		"_channel_B.png": bImg,
		"_channel_A.png": aImg,
	}

	for suffix, channelImg := range channels {
		filename := strings.TrimSuffix(baseName, ".png") + suffix
		outFile, err := os.Create(filename)
		if err != nil {
			return fmt.Errorf("failed to create %s: %w", filename, err)
		}

		err = png.Encode(outFile, channelImg)
		outFile.Close()

		if err != nil {
			return fmt.Errorf("failed to encode %s: %w", filename, err)
		}

		fmt.Printf("✓ Saved debug channel: %s\n", filename)
	}

	return nil
}

func main() {
	// Load .env file
	err := godotenv.Load()
	if err != nil {
		fmt.Println("Warning: .env file not found, continuing without it")
	}

	// Read Roblox cookie from environment (loaded from .env)
	robloxCookie = os.Getenv("ROBLOSECURITY")
	if robloxCookie == "" {
		fmt.Println("Warning: ROBLOSECURITY not set in .env file. Asset downloads may fail.")
	}

	debugFlag := flag.Bool("debug", false, "Save separate R, G, B, A channel images for debugging")
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		fmt.Println("Usage: go run main.go [--debug] <json_file> [output_dir]")
		os.Exit(1)
	}

	jsonFile := args[0]
	outputDir := "assets"

	if len(args) >= 2 {
		outputDir = args[1]
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

	// Build hierarchy
	hierarchy := BuildHierarchy(elements)
	fmt.Printf("Built hierarchy: %d root elements\n", len(hierarchy.Roots))

	// Download assets
	downloadedAssets := make(map[string]bool)
	assetCache := make(map[string]image.Image)

	for _, element := range elements {
		if element.Image != nil && *element.Image != "" {
			assetID := ExtractAssetID(*element.Image)
			if assetID != "" && !downloadedAssets[assetID] {
				fmt.Printf("Downloading image asset: %s\n", assetID)
				assetPath, err := DownloadAsset(assetID, outputDir)
				if err != nil {
					fmt.Printf("Error: %v\n", err)
				} else {
					downloadedAssets[assetID] = true
					if imgFile, err := os.Open(assetPath); err == nil {
						if img, err := png.Decode(imgFile); err == nil {
							assetCache[assetID] = img
						}
						imgFile.Close()
					}
				}
			}
		}
	}

	fmt.Printf("\n✓ Downloaded %d unique assets\n\n", len(downloadedAssets))

	outputPath := "baked_gui.png"
	canvas, err := BakeToImage(hierarchy, outputPath, assetCache)
	if err != nil {
		fmt.Printf("Error baking image: %v\n", err)
		os.Exit(1)
	}

	if *debugFlag {
		fmt.Println("\n🔍 Debug mode: Saving channel images...")
		err = SaveChannelDebugImages(canvas, outputPath)
		if err != nil {
			fmt.Printf("Error saving debug images: %v\n", err)
		}
	}
}
