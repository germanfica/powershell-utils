package main

import (
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type RewriteConfiguration struct {
	SourcePNGPath       string
	DestinationPNGPath  string
	Require640x400      bool
	RequireVisible16    bool
	AllowOverwrite      bool
}

type OperationContext struct {
	OperationID string
}

type SourceImageInfo struct {
	Width                int
	Height               int
	SourcePaletteEntries int
	UsedPaletteIndexes   []uint8
	UsedColors           []RGBColor
	SourcePalettedImage  *image.Paletted
}

type RewritePlan struct {
	DestinationPalette         color.Palette
	SourceIndexToDestIndex     map[uint8]uint8
	DestinationPaletteHexLines []string
}

type RewriteResult struct {
	DestinationPaletteEntries int
	DestinationUsedIndexes    []uint8
	DestinationUsedColors     []RGBColor
}

type RGBColor struct {
	R uint8
	G uint8
	B uint8
}

var FixedPC98Palette = [16]RGBColor{
	{0, 17, 51},
	{17, 0, 51},
	{17, 17, 51},
	{17, 17, 68},
	{34, 17, 85},
	{34, 34, 102},
	{34, 34, 119},
	{51, 34, 119},
	{51, 51, 136},
	{68, 51, 153},
	{85, 68, 153},
	{102, 85, 170},
	{119, 102, 170},
	{136, 119, 170},
	{187, 153, 170},
	{238, 187, 170},
}

func main() {
	OperationContextValue := BuildOperationContext()

	RewriteConfigurationValue, ParseError := ParseFlags()
	if ParseError != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", ParseError)
		os.Exit(2)
	}

	RunError := RunRewriteOperation(OperationContextValue, RewriteConfigurationValue)
	if RunError != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", RunError)
		os.Exit(1)
	}
}

func ParseFlags() (RewriteConfiguration, error) {
	var RewriteConfigurationValue RewriteConfiguration

	flag.StringVar(&RewriteConfigurationValue.SourcePNGPath, "source", "", "Source indexed PNG path")
	flag.StringVar(&RewriteConfigurationValue.DestinationPNGPath, "destination", "", "Destination PNG path")
	flag.BoolVar(&RewriteConfigurationValue.Require640x400, "require-640x400", true, "Require source image to be exactly 640x400")
	flag.BoolVar(&RewriteConfigurationValue.RequireVisible16, "require-visible16", true, "Require source image to visibly use exactly 16 colors")
	flag.BoolVar(&RewriteConfigurationValue.AllowOverwrite, "overwrite", false, "Allow overwriting existing destination file")
	flag.Parse()

	if RewriteConfigurationValue.SourcePNGPath == "" {
		if flag.NArg() >= 1 {
			RewriteConfigurationValue.SourcePNGPath = flag.Arg(0)
		}
	}
	if RewriteConfigurationValue.DestinationPNGPath == "" {
		if flag.NArg() >= 2 {
			RewriteConfigurationValue.DestinationPNGPath = flag.Arg(1)
		}
	}

	if RewriteConfigurationValue.SourcePNGPath == "" {
		return RewriteConfiguration{}, errors.New("missing source PNG path; use -source or pass it as the first positional argument")
	}

	if RewriteConfigurationValue.DestinationPNGPath == "" {
		RewriteConfigurationValue.DestinationPNGPath = BuildDefaultDestinationPNGPath(RewriteConfigurationValue.SourcePNGPath)
	}

	return RewriteConfigurationValue, nil
}

func RunRewriteOperation(OperationContextValue OperationContext, RewriteConfigurationValue RewriteConfiguration) error {
	fmt.Println("PC-98 PNG Palette Rewriter")
	fmt.Printf("OperationID: %s\n", OperationContextValue.OperationID)
	fmt.Printf("Source:      %s\n", RewriteConfigurationValue.SourcePNGPath)
	fmt.Printf("Destination: %s\n", RewriteConfigurationValue.DestinationPNGPath)
	fmt.Printf("Rules:       require-640x400=%t, require-visible16=%t\n", RewriteConfigurationValue.Require640x400, RewriteConfigurationValue.RequireVisible16)
	fmt.Println()

	ValidationError := ValidateRewriteConfiguration(RewriteConfigurationValue)
	if ValidationError != nil {
		return ValidationError
	}

	SourceImageInfoValue, LoadError := LoadSourcePalettedPNG(RewriteConfigurationValue.SourcePNGPath)
	if LoadError != nil {
		return LoadError
	}

	SourceValidationError := ValidateSourceImageAgainstRules(SourceImageInfoValue, RewriteConfigurationValue)
	if SourceValidationError != nil {
		return SourceValidationError
	}

	RewritePlanValue, PlanError := BuildRewritePlan(SourceImageInfoValue)
	if PlanError != nil {
		return PlanError
	}

	PrintSourceSummary(SourceImageInfoValue)
	PrintRewritePlanSummary(RewritePlanValue)

	DestinationPalettedImage, ImageBuildError := BuildDestinationPalettedImage(SourceImageInfoValue, RewritePlanValue)
	if ImageBuildError != nil {
		return ImageBuildError
	}

	DestinationValidationError := ValidateDestinationPalettedImage(DestinationPalettedImage, RewriteConfigurationValue)
	if DestinationValidationError != nil {
		return DestinationValidationError
	}

	WriteLogHeaderBeforeSaving()
	SaveError := SaveDestinationPNG(RewriteConfigurationValue.DestinationPNGPath, DestinationPalettedImage, RewriteConfigurationValue.AllowOverwrite)
	if SaveError != nil {
		return SaveError
	}

	RewriteResultValue := BuildRewriteResult(DestinationPalettedImage)
	PrintRewriteResultSummary(RewriteResultValue)
	fmt.Println("Result: OK")

	return nil
}

func BuildOperationContext() OperationContext {
	Timestamp := time.Now().UTC().Format("20060102T150405Z")
	ProcessID := os.Getpid()
	OperationID := fmt.Sprintf("%s-%d", Timestamp, ProcessID)
	return OperationContext{OperationID: OperationID}
}

func BuildDefaultDestinationPNGPath(SourcePNGPath string) string {
	SourceExtension := filepath.Ext(SourcePNGPath)
	SourceBaseName := strings.TrimSuffix(SourcePNGPath, SourceExtension)
	if SourceExtension == "" {
		return SourceBaseName + "_pc98_rewritten.png"
	}
	return SourceBaseName + "_pc98_rewritten" + SourceExtension
}

func ValidateRewriteConfiguration(RewriteConfigurationValue RewriteConfiguration) error {
	if RewriteConfigurationValue.SourcePNGPath == "" {
		return errors.New("source PNG path is empty")
	}
	if RewriteConfigurationValue.DestinationPNGPath == "" {
		return errors.New("destination PNG path is empty")
	}

	SourceAbsolutePath, SourceAbsoluteError := filepath.Abs(RewriteConfigurationValue.SourcePNGPath)
	if SourceAbsoluteError != nil {
		return fmt.Errorf("resolve source absolute path: %w", SourceAbsoluteError)
	}
	DestinationAbsolutePath, DestinationAbsoluteError := filepath.Abs(RewriteConfigurationValue.DestinationPNGPath)
	if DestinationAbsoluteError != nil {
		return fmt.Errorf("resolve destination absolute path: %w", DestinationAbsoluteError)
	}
	if SourceAbsolutePath == DestinationAbsolutePath {
		return errors.New("source and destination paths are the same; refusing to rewrite in place")
	}

	SourceFileInfo, SourceStatError := os.Stat(RewriteConfigurationValue.SourcePNGPath)
	if SourceStatError != nil {
		return fmt.Errorf("stat source PNG: %w", SourceStatError)
	}
	if SourceFileInfo.IsDir() {
		return errors.New("source path points to a directory, not a PNG file")
	}

	DestinationDirectoryPath := filepath.Dir(RewriteConfigurationValue.DestinationPNGPath)
	DestinationDirectoryInfo, DestinationDirectoryError := os.Stat(DestinationDirectoryPath)
	if DestinationDirectoryError != nil {
		return fmt.Errorf("stat destination directory: %w", DestinationDirectoryError)
	}
	if !DestinationDirectoryInfo.IsDir() {
		return errors.New("destination directory path is not a directory")
	}

	DestinationFileInfo, DestinationStatError := os.Stat(RewriteConfigurationValue.DestinationPNGPath)
	if DestinationStatError == nil {
		if DestinationFileInfo.IsDir() {
			return errors.New("destination path points to a directory")
		}
		if !RewriteConfigurationValue.AllowOverwrite {
			return errors.New("destination file already exists; use -overwrite to allow replacing it")
		}
	} else {
		if !errors.Is(DestinationStatError, os.ErrNotExist) {
			return fmt.Errorf("stat destination PNG: %w", DestinationStatError)
		}
	}

	return nil
}

func LoadSourcePalettedPNG(SourcePNGPath string) (SourceImageInfo, error) {
	SourceFile, OpenError := os.Open(SourcePNGPath)
	if OpenError != nil {
		return SourceImageInfo{}, fmt.Errorf("open source PNG: %w", OpenError)
	}
	defer SourceFile.Close()

	DecodedImage, DecodeError := png.Decode(SourceFile)
	if DecodeError != nil {
		return SourceImageInfo{}, fmt.Errorf("decode source PNG: %w", DecodeError)
	}

	SourcePalettedImage, TypeOK := DecodedImage.(*image.Paletted)
	if !TypeOK {
		return SourceImageInfo{}, errors.New("source PNG is not indexed; decoded image is not *image.Paletted")
	}

	Bounds := SourcePalettedImage.Bounds()
	UsedPaletteIndexes := ExtractUsedPaletteIndexes(SourcePalettedImage)
	UsedColors, UsedColorsError := BuildUsedColorsFromIndexes(SourcePalettedImage.Palette, UsedPaletteIndexes)
	if UsedColorsError != nil {
		return SourceImageInfo{}, UsedColorsError
	}

	SourceImageInfoValue := SourceImageInfo{
		Width:                Bounds.Dx(),
		Height:               Bounds.Dy(),
		SourcePaletteEntries: len(SourcePalettedImage.Palette),
		UsedPaletteIndexes:   UsedPaletteIndexes,
		UsedColors:           UsedColors,
		SourcePalettedImage:  SourcePalettedImage,
	}
	return SourceImageInfoValue, nil
}

func ExtractUsedPaletteIndexes(SourcePalettedImage *image.Paletted) []uint8 {
	UsedIndexMap := make(map[uint8]bool)
	for PixelOffsetIndex := 0; PixelOffsetIndex < len(SourcePalettedImage.Pix); PixelOffsetIndex++ {
		PaletteIndex := SourcePalettedImage.Pix[PixelOffsetIndex]
		UsedIndexMap[PaletteIndex] = true
	}

	UsedPaletteIndexes := make([]uint8, 0, len(UsedIndexMap))
	for PaletteIndex := range UsedIndexMap {
		UsedPaletteIndexes = append(UsedPaletteIndexes, PaletteIndex)
	}
	sort.Slice(UsedPaletteIndexes, func(LeftIndex int, RightIndex int) bool {
		return UsedPaletteIndexes[LeftIndex] < UsedPaletteIndexes[RightIndex]
	})
	return UsedPaletteIndexes
}

func BuildUsedColorsFromIndexes(SourcePalette color.Palette, UsedPaletteIndexes []uint8) ([]RGBColor, error) {
	UsedColors := make([]RGBColor, 0, len(UsedPaletteIndexes))
	for PaletteIndexOffset := 0; PaletteIndexOffset < len(UsedPaletteIndexes); PaletteIndexOffset++ {
		PaletteIndex := UsedPaletteIndexes[PaletteIndexOffset]
		if int(PaletteIndex) >= len(SourcePalette) {
			return nil, fmt.Errorf("used palette index %d exceeds source palette size %d", PaletteIndex, len(SourcePalette))
		}
		UsedColors = append(UsedColors, ConvertColorToRGBColor(SourcePalette[PaletteIndex]))
	}
	return UsedColors, nil
}

func ValidateSourceImageAgainstRules(SourceImageInfoValue SourceImageInfo, RewriteConfigurationValue RewriteConfiguration) error {
	if RewriteConfigurationValue.Require640x400 {
		if SourceImageInfoValue.Width != 640 || SourceImageInfoValue.Height != 400 {
			return fmt.Errorf("source image size is %dx%d; expected exactly 640x400", SourceImageInfoValue.Width, SourceImageInfoValue.Height)
		}
	}
	if RewriteConfigurationValue.RequireVisible16 {
		if len(SourceImageInfoValue.UsedPaletteIndexes) != 16 {
			return fmt.Errorf("source image visibly uses %d colors; expected exactly 16", len(SourceImageInfoValue.UsedPaletteIndexes))
		}
	} else {
		if len(SourceImageInfoValue.UsedPaletteIndexes) > 16 {
			return fmt.Errorf("source image visibly uses %d colors; expected at most 16", len(SourceImageInfoValue.UsedPaletteIndexes))
		}
	}

	for UsedColorIndex := 0; UsedColorIndex < len(SourceImageInfoValue.UsedColors); UsedColorIndex++ {
		UsedColor := SourceImageInfoValue.UsedColors[UsedColorIndex]
		if !IsRGB12Color(UsedColor) {
			return fmt.Errorf("source image uses non-RGB12 color %s", FormatRGBColorHex(UsedColor))
		}
		if !PaletteContainsColor(FixedPC98Palette[:], UsedColor) {
			return fmt.Errorf("source image uses color %s which is not present in the fixed 16-color destination palette", FormatRGBColorHex(UsedColor))
		}
	}

	return nil
}

func BuildRewritePlan(SourceImageInfoValue SourceImageInfo) (RewritePlan, error) {
	DestinationPalette := BuildDestinationPalette()
	SourceIndexToDestIndex := make(map[uint8]uint8)

	DestinationPaletteHexLines := make([]string, 0, len(DestinationPalette))
	for DestinationPaletteIndex := 0; DestinationPaletteIndex < len(FixedPC98Palette); DestinationPaletteIndex++ {
		DestinationPaletteHexLines = append(DestinationPaletteHexLines, FormatRGBColorHex(FixedPC98Palette[DestinationPaletteIndex]))
	}

	for UsedPaletteIndexOffset := 0; UsedPaletteIndexOffset < len(SourceImageInfoValue.UsedPaletteIndexes); UsedPaletteIndexOffset++ {
		SourcePaletteIndex := SourceImageInfoValue.UsedPaletteIndexes[UsedPaletteIndexOffset]
		SourceColor := SourceImageInfoValue.UsedColors[UsedPaletteIndexOffset]
		DestinationPaletteIndex := FindDestinationPaletteIndex(FixedPC98Palette[:], SourceColor)
		if DestinationPaletteIndex < 0 {
			return RewritePlan{}, fmt.Errorf("cannot map source palette index %d color %s into destination palette", SourcePaletteIndex, FormatRGBColorHex(SourceColor))
		}
		SourceIndexToDestIndex[SourcePaletteIndex] = uint8(DestinationPaletteIndex)
	}

	RewritePlanValue := RewritePlan{
		DestinationPalette:         DestinationPalette,
		SourceIndexToDestIndex:     SourceIndexToDestIndex,
		DestinationPaletteHexLines: DestinationPaletteHexLines,
	}
	return RewritePlanValue, nil
}

func BuildDestinationPalette() color.Palette {
	DestinationPalette := make(color.Palette, 0, len(FixedPC98Palette))
	for PaletteIndex := 0; PaletteIndex < len(FixedPC98Palette); PaletteIndex++ {
		PaletteColor := FixedPC98Palette[PaletteIndex]
		DestinationPalette = append(DestinationPalette, color.RGBA{R: PaletteColor.R, G: PaletteColor.G, B: PaletteColor.B, A: 255})
	}
	return DestinationPalette
}

func BuildDestinationPalettedImage(SourceImageInfoValue SourceImageInfo, RewritePlanValue RewritePlan) (*image.Paletted, error) {
	Bounds := SourceImageInfoValue.SourcePalettedImage.Bounds()
	DestinationPalettedImage := image.NewPaletted(Bounds, RewritePlanValue.DestinationPalette)

	if len(DestinationPalettedImage.Pix) != len(SourceImageInfoValue.SourcePalettedImage.Pix) {
		return nil, errors.New("destination pixel buffer length does not match source pixel buffer length")
	}

	for PixelOffsetIndex := 0; PixelOffsetIndex < len(SourceImageInfoValue.SourcePalettedImage.Pix); PixelOffsetIndex++ {
		SourcePaletteIndex := SourceImageInfoValue.SourcePalettedImage.Pix[PixelOffsetIndex]
		DestinationPaletteIndex, Exists := RewritePlanValue.SourceIndexToDestIndex[SourcePaletteIndex]
		if !Exists {
			SourceColor := ConvertColorToRGBColor(SourceImageInfoValue.SourcePalettedImage.Palette[SourcePaletteIndex])
			return nil, fmt.Errorf("no destination palette mapping for source palette index %d color %s", SourcePaletteIndex, FormatRGBColorHex(SourceColor))
		}
		DestinationPalettedImage.Pix[PixelOffsetIndex] = DestinationPaletteIndex
	}

	return DestinationPalettedImage, nil
}

func ValidateDestinationPalettedImage(DestinationPalettedImage *image.Paletted, RewriteConfigurationValue RewriteConfiguration) error {
	Bounds := DestinationPalettedImage.Bounds()
	if RewriteConfigurationValue.Require640x400 {
		if Bounds.Dx() != 640 || Bounds.Dy() != 400 {
			return fmt.Errorf("destination image size is %dx%d; expected 640x400", Bounds.Dx(), Bounds.Dy())
		}
	}
	if len(DestinationPalettedImage.Palette) != 16 {
		return fmt.Errorf("destination palette has %d entries; expected exactly 16", len(DestinationPalettedImage.Palette))
	}

	UsedPaletteIndexes := ExtractUsedPaletteIndexes(DestinationPalettedImage)
	if RewriteConfigurationValue.RequireVisible16 {
		if len(UsedPaletteIndexes) != 16 {
			return fmt.Errorf("destination image visibly uses %d colors; expected exactly 16", len(UsedPaletteIndexes))
		}
	} else {
		if len(UsedPaletteIndexes) > 16 {
			return fmt.Errorf("destination image visibly uses %d colors; expected at most 16", len(UsedPaletteIndexes))
		}
	}

	for PaletteEntryIndex := 0; PaletteEntryIndex < len(DestinationPalettedImage.Palette); PaletteEntryIndex++ {
		PaletteColor := ConvertColorToRGBColor(DestinationPalettedImage.Palette[PaletteEntryIndex])
		if !IsRGB12Color(PaletteColor) {
			return fmt.Errorf("destination palette index %d is not RGB12: %s", PaletteEntryIndex, FormatRGBColorHex(PaletteColor))
		}
	}

	return nil
}

func SaveDestinationPNG(DestinationPNGPath string, DestinationPalettedImage *image.Paletted, AllowOverwrite bool) error {
	DestinationOpenFlags := os.O_CREATE | os.O_WRONLY
	if AllowOverwrite {
		DestinationOpenFlags = DestinationOpenFlags | os.O_TRUNC
	} else {
		DestinationOpenFlags = DestinationOpenFlags | os.O_EXCL
	}

	DestinationFile, CreateError := os.OpenFile(DestinationPNGPath, DestinationOpenFlags, 0644)
	if CreateError != nil {
		return fmt.Errorf("create destination PNG: %w", CreateError)
	}

	EncodeError := png.Encode(DestinationFile, DestinationPalettedImage)
	CloseError := DestinationFile.Close()
	if EncodeError != nil {
		_ = os.Remove(DestinationPNGPath)
		return fmt.Errorf("encode destination PNG: %w", EncodeError)
	}
	if CloseError != nil {
		_ = os.Remove(DestinationPNGPath)
		return fmt.Errorf("close destination PNG: %w", CloseError)
	}

	return nil
}

func BuildRewriteResult(DestinationPalettedImage *image.Paletted) RewriteResult {
	DestinationUsedIndexes := ExtractUsedPaletteIndexes(DestinationPalettedImage)
	DestinationUsedColors := make([]RGBColor, 0, len(DestinationUsedIndexes))
	for UsedPaletteOffset := 0; UsedPaletteOffset < len(DestinationUsedIndexes); UsedPaletteOffset++ {
		PaletteIndex := DestinationUsedIndexes[UsedPaletteOffset]
		DestinationUsedColors = append(DestinationUsedColors, ConvertColorToRGBColor(DestinationPalettedImage.Palette[PaletteIndex]))
	}
	return RewriteResult{
		DestinationPaletteEntries: len(DestinationPalettedImage.Palette),
		DestinationUsedIndexes:    DestinationUsedIndexes,
		DestinationUsedColors:     DestinationUsedColors,
	}
}

func PrintSourceSummary(SourceImageInfoValue SourceImageInfo) {
	fmt.Println("Source summary:")
	fmt.Printf("  Size:                  %dx%d\n", SourceImageInfoValue.Width, SourceImageInfoValue.Height)
	fmt.Printf("  Palette entries:       %d\n", SourceImageInfoValue.SourcePaletteEntries)
	fmt.Printf("  Visible palette slots: %d\n", len(SourceImageInfoValue.UsedPaletteIndexes))
	fmt.Println("  Visible colors:")
	for UsedColorIndex := 0; UsedColorIndex < len(SourceImageInfoValue.UsedColors); UsedColorIndex++ {
		PaletteIndex := SourceImageInfoValue.UsedPaletteIndexes[UsedColorIndex]
		UsedColor := SourceImageInfoValue.UsedColors[UsedColorIndex]
		fmt.Printf("    source[%3d] = %s\n", PaletteIndex, FormatRGBColorHex(UsedColor))
	}
	fmt.Println()
}

func PrintRewritePlanSummary(RewritePlanValue RewritePlan) {
	fmt.Println("Destination palette (fixed 16 entries):")
	for PaletteIndex := 0; PaletteIndex < len(RewritePlanValue.DestinationPaletteHexLines); PaletteIndex++ {
		fmt.Printf("  dest[%2d] = %s\n", PaletteIndex, RewritePlanValue.DestinationPaletteHexLines[PaletteIndex])
	}
	fmt.Println()
}

func PrintRewriteResultSummary(RewriteResultValue RewriteResult) {
	fmt.Println()
	fmt.Println("Destination summary:")
	fmt.Printf("  Palette entries:       %d\n", RewriteResultValue.DestinationPaletteEntries)
	fmt.Printf("  Visible palette slots: %d\n", len(RewriteResultValue.DestinationUsedIndexes))
	fmt.Println("  Visible colors:")
	for UsedColorIndex := 0; UsedColorIndex < len(RewriteResultValue.DestinationUsedColors); UsedColorIndex++ {
		PaletteIndex := RewriteResultValue.DestinationUsedIndexes[UsedColorIndex]
		UsedColor := RewriteResultValue.DestinationUsedColors[UsedColorIndex]
		fmt.Printf("    dest[%2d] = %s\n", PaletteIndex, FormatRGBColorHex(UsedColor))
	}
}

func WriteLogHeaderBeforeSaving() {
	fmt.Println("Validation complete. Writing destination PNG...")
}

func ConvertColorToRGBColor(SourceColor color.Color) RGBColor {
	Red16, Green16, Blue16, _ := SourceColor.RGBA()
	return RGBColor{R: uint8(Red16 >> 8), G: uint8(Green16 >> 8), B: uint8(Blue16 >> 8)}
}

func IsRGB12Color(SourceColor RGBColor) bool {
	if SourceColor.R%17 != 0 {
		return false
	}
	if SourceColor.G%17 != 0 {
		return false
	}
	if SourceColor.B%17 != 0 {
		return false
	}
	return true
}

func PaletteContainsColor(PaletteColors []RGBColor, TargetColor RGBColor) bool {
	for PaletteIndex := 0; PaletteIndex < len(PaletteColors); PaletteIndex++ {
		if PaletteColors[PaletteIndex] == TargetColor {
			return true
		}
	}
	return false
}

func FindDestinationPaletteIndex(PaletteColors []RGBColor, TargetColor RGBColor) int {
	for PaletteIndex := 0; PaletteIndex < len(PaletteColors); PaletteIndex++ {
		if PaletteColors[PaletteIndex] == TargetColor {
			return PaletteIndex
		}
	}
	return -1
}

func FormatRGBColorHex(SourceColor RGBColor) string {
	return fmt.Sprintf("#%02X%02X%02X", SourceColor.R, SourceColor.G, SourceColor.B)
}
