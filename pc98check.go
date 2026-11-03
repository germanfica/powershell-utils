// pc98check validates a PNG against a 640x400, 16-color, RGB-12 palette profile.
// It never modifies the source file. Compatible with Go 1.17.
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"strings"
)

const (
	RequiredWidth       = 640
	RequiredHeight      = 400
	MaximumColors       = 16
	PNGIndexedColorType = 3
	RGB12ChannelStep    = 17
)

var pngSignature = []byte{137, 80, 78, 71, 13, 10, 26, 10}

type Configuration struct {
	SourcePNGPath string
	Mode          string
}

type SourcePNGMetadata struct {
	Width                int
	Height               int
	BitDepth             uint8
	ColorType            uint8
	PaletteEntries       int
	HasTransparencyChunk bool
}

type SourcePNGInspection struct {
	Metadata            SourcePNGMetadata
	UsedPaletteEntries  int
	UniqueVisibleColors int
	InvalidRGB12Entries int
	HasTransparentPixel bool
	IsIndexed           bool
}

type ValidationCheck struct {
	Name     string
	Actual   string
	Expected string
	Passed   bool
}

type ValidationResult struct {
	Checks []ValidationCheck
	Valid  bool
}

func main() {
	configuration, err := ParseConfiguration()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(2)
	}

	inspection, err := InspectSourcePNG(configuration.SourcePNGPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error inspecting source PNG:", err)
		os.Exit(2)
	}

	result := ValidateSourcePNG(inspection, configuration.Mode)
	PrintValidationResult(configuration, inspection, result)
	if !result.Valid {
		os.Exit(1)
	}
}

func ParseConfiguration() (Configuration, error) {
	configuration := Configuration{}
	flag.StringVar(&configuration.SourcePNGPath, "source", "", "path to the source PNG (read-only)")
	flag.StringVar(&configuration.Mode, "mode", "strict", "validation mode: strict or compatible")
	flag.Parse()

	if flag.NArg() > 1 {
		return configuration, fmt.Errorf("expected one source PNG path, not %d positional arguments", flag.NArg())
	}
	if flag.NArg() == 1 {
		if configuration.SourcePNGPath != "" {
			return configuration, fmt.Errorf("specify the source PNG using -source or a positional path, not both")
		}
		configuration.SourcePNGPath = flag.Arg(0)
	}
	if strings.TrimSpace(configuration.SourcePNGPath) == "" {
		return configuration, fmt.Errorf("missing source PNG; usage: pc98check [-mode strict|compatible] -source image.png")
	}
	if configuration.Mode != "strict" && configuration.Mode != "compatible" {
		return configuration, fmt.Errorf("invalid mode %q; expected strict or compatible", configuration.Mode)
	}
	return configuration, nil
}

func InspectSourcePNG(SourcePNGPath string) (SourcePNGInspection, error) {
	inspection := SourcePNGInspection{}

	metadata, err := ReadSourcePNGMetadata(SourcePNGPath)
	if err != nil {
		return inspection, err
	}
	inspection.Metadata = metadata

	sourceFile, err := os.Open(SourcePNGPath)
	if err != nil {
		return inspection, fmt.Errorf("open %q: %w", SourcePNGPath, err)
	}
	defer sourceFile.Close()

	decodedImage, err := png.Decode(sourceFile)
	if err != nil {
		return inspection, fmt.Errorf("decode %q: %w", SourcePNGPath, err)
	}

	paletteImage, isIndexed := decodedImage.(*image.Paletted)
	inspection.IsIndexed = isIndexed
	if isIndexed {
		if len(paletteImage.Palette) != metadata.PaletteEntries {
			return inspection, fmt.Errorf("PNG palette has inconsistent entry counts")
		}

		for _, paletteColor := range paletteImage.Palette {
			convertedColor := color.NRGBAModel.Convert(paletteColor).(color.NRGBA)
			if convertedColor.R%RGB12ChannelStep != 0 ||
				convertedColor.G%RGB12ChannelStep != 0 ||
				convertedColor.B%RGB12ChannelStep != 0 {
				inspection.InvalidRGB12Entries++
			}
		}

		usedIndexes := make([]bool, len(paletteImage.Palette))
		visibleColors := make(map[uint32]bool)
		bounds := paletteImage.Bounds()
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				paletteIndex := int(paletteImage.ColorIndexAt(x, y))
				if paletteIndex >= len(usedIndexes) {
					return inspection, fmt.Errorf("pixel at (%d,%d) references missing palette entry %d", x, y, paletteIndex)
				}
				usedIndexes[paletteIndex] = true
				pixelColor := color.NRGBAModel.Convert(paletteImage.Palette[paletteIndex]).(color.NRGBA)
				if pixelColor.A != 255 {
					inspection.HasTransparentPixel = true
				}
				visibleColors[PackRGBA(pixelColor)] = true
			}
		}
		for _, used := range usedIndexes {
			if used {
				inspection.UsedPaletteEntries++
			}
		}
		inspection.UniqueVisibleColors = len(visibleColors)
		return inspection, nil
	}

	// Still count visible colors in a non-indexed PNG so the report is useful.
	visibleColors := make(map[uint32]bool)
	bounds := decodedImage.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			pixelColor := color.NRGBAModel.Convert(decodedImage.At(x, y)).(color.NRGBA)
			if pixelColor.A != 255 {
				inspection.HasTransparentPixel = true
			}
			visibleColors[PackRGBA(pixelColor)] = true
		}
	}
	inspection.UniqueVisibleColors = len(visibleColors)
	return inspection, nil
}

func PackRGBA(pixelColor color.NRGBA) uint32 {
	return uint32(pixelColor.R)<<24 |
		uint32(pixelColor.G)<<16 |
		uint32(pixelColor.B)<<8 |
		uint32(pixelColor.A)
}

func ReadSourcePNGMetadata(SourcePNGPath string) (SourcePNGMetadata, error) {
	metadata := SourcePNGMetadata{}
	sourceFile, err := os.Open(SourcePNGPath)
	if err != nil {
		return metadata, fmt.Errorf("open %q: %w", SourcePNGPath, err)
	}
	defer sourceFile.Close()

	fileInfo, err := sourceFile.Stat()
	if err != nil {
		return metadata, fmt.Errorf("stat %q: %w", SourcePNGPath, err)
	}
	if !fileInfo.Mode().IsRegular() {
		return metadata, fmt.Errorf("%q is not a regular file", SourcePNGPath)
	}

	signature := make([]byte, len(pngSignature))
	if _, err := io.ReadFull(sourceFile, signature); err != nil {
		return metadata, fmt.Errorf("read PNG signature: %w", err)
	}
	if !bytes.Equal(signature, pngSignature) {
		return metadata, fmt.Errorf("%q is not a PNG file", SourcePNGPath)
	}

	foundIHDR := false
	foundPLTE := false
	foundIEND := false

	for !foundIEND {
		var chunkHeader [8]byte
		if _, err := io.ReadFull(sourceFile, chunkHeader[:]); err != nil {
			return metadata, fmt.Errorf("read PNG chunk header: %w", err)
		}
		chunkLength := int64(binary.BigEndian.Uint32(chunkHeader[0:4]))
		chunkType := string(chunkHeader[4:8])

		chunkDataPosition, err := sourceFile.Seek(0, io.SeekCurrent)
		if err != nil {
			return metadata, fmt.Errorf("get PNG chunk position: %w", err)
		}
		if chunkLength+4 > fileInfo.Size()-chunkDataPosition {
			return metadata, fmt.Errorf("truncated PNG chunk %q", chunkType)
		}
		if !foundIHDR && chunkType != "IHDR" {
			return metadata, fmt.Errorf("PNG must start with an IHDR chunk")
		}

		switch chunkType {
		case "IHDR":
			if foundIHDR || chunkLength != 13 {
				return metadata, fmt.Errorf("invalid or duplicate IHDR chunk")
			}
			var headerData [13]byte
			if _, err := io.ReadFull(sourceFile, headerData[:]); err != nil {
				return metadata, fmt.Errorf("read IHDR data: %w", err)
			}
			metadata.Width = int(binary.BigEndian.Uint32(headerData[0:4]))
			metadata.Height = int(binary.BigEndian.Uint32(headerData[4:8]))
			metadata.BitDepth = headerData[8]
			metadata.ColorType = headerData[9]
			foundIHDR = true
		case "PLTE":
			if foundPLTE || chunkLength == 0 || chunkLength%3 != 0 {
				return metadata, fmt.Errorf("invalid or duplicate PLTE chunk")
			}
			metadata.PaletteEntries = int(chunkLength / 3)
			foundPLTE = true
			if _, err := sourceFile.Seek(chunkLength, io.SeekCurrent); err != nil {
				return metadata, fmt.Errorf("skip PLTE data: %w", err)
			}
		case "tRNS":
			metadata.HasTransparencyChunk = true
			if _, err := sourceFile.Seek(chunkLength, io.SeekCurrent); err != nil {
				return metadata, fmt.Errorf("skip tRNS data: %w", err)
			}
		case "IEND":
			if chunkLength != 0 {
				return metadata, fmt.Errorf("invalid IEND chunk length")
			}
			foundIEND = true
		default:
			if _, err := sourceFile.Seek(chunkLength, io.SeekCurrent); err != nil {
				return metadata, fmt.Errorf("skip PNG chunk %q: %w", chunkType, err)
			}
		}

		var chunkCRC [4]byte
		if _, err := io.ReadFull(sourceFile, chunkCRC[:]); err != nil {
			return metadata, fmt.Errorf("read CRC for PNG chunk %q: %w", chunkType, err)
		}
	}

	endPosition, err := sourceFile.Seek(0, io.SeekCurrent)
	if err != nil {
		return metadata, fmt.Errorf("get PNG end position: %w", err)
	}
	if endPosition != fileInfo.Size() {
		return metadata, fmt.Errorf("PNG has %d unexpected bytes after IEND", fileInfo.Size()-endPosition)
	}
	return metadata, nil
}

func ValidateSourcePNG(inspection SourcePNGInspection, mode string) ValidationResult {
	result := ValidationResult{Valid: true}
	metadata := inspection.Metadata

	result.Checks = append(result.Checks, ValidationCheck{
		Name: "Resolution", Actual: fmt.Sprintf("%dx%d", metadata.Width, metadata.Height),
		Expected: "640x400", Passed: metadata.Width == RequiredWidth && metadata.Height == RequiredHeight,
	})
	result.Checks = append(result.Checks, ValidationCheck{
		Name: "PNG format", Actual: fmt.Sprintf("color type %d, bit depth %d", metadata.ColorType, metadata.BitDepth),
		Expected: "indexed PNG (color type 3)", Passed: inspection.IsIndexed && metadata.ColorType == PNGIndexedColorType,
	})

	paletteOK := metadata.PaletteEntries == MaximumColors
	colorsOK := inspection.UniqueVisibleColors == MaximumColors
	paletteExpected := "exactly 16"
	colorsExpected := "exactly 16"
	if mode == "compatible" {
		paletteOK = metadata.PaletteEntries >= 1 && metadata.PaletteEntries <= MaximumColors
		colorsOK = inspection.UniqueVisibleColors >= 1 && inspection.UniqueVisibleColors <= MaximumColors
		paletteExpected = "1 to 16"
		colorsExpected = "1 to 16"
	}
	result.Checks = append(result.Checks, ValidationCheck{
		Name: "Palette entries", Actual: fmt.Sprintf("%d", metadata.PaletteEntries),
		Expected: paletteExpected, Passed: inspection.IsIndexed && paletteOK,
	})
	result.Checks = append(result.Checks, ValidationCheck{
		Name: "Visible colors", Actual: fmt.Sprintf("%d", inspection.UniqueVisibleColors),
		Expected: colorsExpected, Passed: colorsOK,
	})
	result.Checks = append(result.Checks, ValidationCheck{
		Name: "RGB12 palette", Actual: fmt.Sprintf("%d non-RGB12 entries", inspection.InvalidRGB12Entries),
		Expected: "all channels are multiples of 17", Passed: inspection.IsIndexed && inspection.InvalidRGB12Entries == 0,
	})
	transparency := "none"
	if metadata.HasTransparencyChunk || inspection.HasTransparentPixel {
		transparency = "present"
	}
	result.Checks = append(result.Checks, ValidationCheck{
		Name: "Transparency", Actual: transparency,
		Expected: "none (no tRNS chunk or transparent pixels)", Passed: !metadata.HasTransparencyChunk && !inspection.HasTransparentPixel,
	})

	for _, check := range result.Checks {
		if !check.Passed {
			result.Valid = false
		}
	}
	return result
}

func PrintValidationResult(configuration Configuration, inspection SourcePNGInspection, result ValidationResult) {
	fmt.Println("PC-98 PNG Validator")
	fmt.Println("Source:", configuration.SourcePNGPath)
	fmt.Println("Mode:  ", configuration.Mode)
	fmt.Println()
	for _, check := range result.Checks {
		status := "PASS"
		if !check.Passed {
			status = "FAIL"
		}
		fmt.Printf("%-17s %-4s  %s (expected: %s)\n", check.Name+":", status, check.Actual, check.Expected)
	}
	if inspection.IsIndexed {
		fmt.Printf("Used palette entries: %d of %d\n", inspection.UsedPaletteEntries, inspection.Metadata.PaletteEntries)
	}
	fmt.Println()
	if result.Valid {
		fmt.Println("Result: VALID")
	} else {
		fmt.Println("Result: INVALID")
	}
}
