// extract_all_mkv.go
// Extrae el contenido de archivos MKV y analiza los archivos extraidos con ClamAV.
// Compatible con Go 1.17. Solo usa la biblioteca estandar de Go.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	DefaultDestinationDirectory = "extracted-all"
	MaxClamFileSize             = "512M"
	MaxClamScanSize             = "1024M"
)

type Configuration struct {
	SourceDirectory      string
	DestinationDirectory string
	SourceMKVPaths       []string
	SkipScan             bool
	ScanVideo            bool
	SkipVideoExtraction  bool
	OperationID          string
}

type MKVIdentification struct {
	Tracks      []MKVTrack      `json:"tracks"`
	Attachments []MKVAttachment `json:"attachments"`
}

type MKVTrack struct {
	ID         int    `json:"id"`
	Type       string `json:"type"`
	Properties struct {
		CodecID string `json:"codec_id"`
	} `json:"properties"`
}

type MKVAttachment struct {
	ID       int    `json:"id"`
	FileName string `json:"file_name"`
}

type TrackExtraction struct {
	ID                  int
	Type                string
	CodecID             string
	DestinationFilePath string
	UseRawExtraction    bool
}

type AttachmentExtraction struct {
	ID                  int
	DestinationFilePath string
}

type FileOperation struct {
	SourceMKVPath        string
	DestinationDirectory string
	IdentificationJSON   []byte
	Tracks               []TrackExtraction
	Attachments          []AttachmentExtraction
	TimestampTrackIDs    []int
}

type OperationLogger struct {
	File *os.File
}

func main() {
	if err := Execute(); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}

func Execute() error {
	configuration, err := ReadConfiguration(os.Args[1:])
	if err != nil {
		return err
	}
	if err := ValidateDependencies(configuration); err != nil {
		return err
	}

	sourceMKVPaths, err := FindSourceMKVPaths(configuration)
	if err != nil {
		return err
	}
	operations, err := PrepareOperations(configuration, sourceMKVPaths)
	if err != nil {
		return err
	}

	// Toda la identificacion y la validacion se realiza antes de crear archivos.
	logger, err := CreateOperationLog(configuration, operations)
	if err != nil {
		return err
	}
	defer logger.File.Close()

	for _, operation := range operations {
		if err := ExtractFromSourceMKVToDestination(operation, logger); err != nil {
			logger.Log(fmt.Sprintf("FAILED source=%q error=%q", operation.SourceMKVPath, err))
			return err
		}
		if !configuration.SkipScan {
			if err := ScanExtractedFilesWithClamAV(operation, configuration.ScanVideo, logger); err != nil {
				logger.Log(fmt.Sprintf("SCAN FAILED source=%q error=%q", operation.SourceMKVPath, err))
				return err
			}
		}
	}
	logger.Log(fmt.Sprintf("COMPLETED operation=%s files=%d", configuration.OperationID, len(operations)))
	fmt.Println("Proceso terminado. OperationID:", configuration.OperationID)
	return nil
}

func ReadConfiguration(arguments []string) (Configuration, error) {
	configuration := Configuration{}
	flagSet := flag.NewFlagSet("extract_all_mkv", flag.ContinueOnError)
	flagSet.SetOutput(os.Stderr)
	flagSet.StringVar(&configuration.SourceDirectory, "source-dir", ".", "Directorio con archivos .mkv (solo nivel superior)")
	flagSet.StringVar(&configuration.DestinationDirectory, "destination-dir", DefaultDestinationDirectory, "Directorio donde guardar las extracciones")
	flagSet.BoolVar(&configuration.SkipScan, "no-scan", false, "Extraer sin ejecutar ClamAV")
	flagSet.BoolVar(&configuration.ScanVideo, "scan-video", false, "Incluir pistas de video en el analisis ClamAV")
	flagSet.BoolVar(&configuration.SkipVideoExtraction, "no-video-extraction", false, "No extraer pistas de video (ahorra espacio)")
	flagSet.Usage = func() {
		fmt.Fprintln(flagSet.Output(), "Uso: go run extract_all_mkv.go [opciones] [archivo1.mkv archivo2.mkv ...]")
		fmt.Fprintln(flagSet.Output(), "Sin archivos explicitos se procesan los .mkv de -source-dir.")
		fmt.Fprintln(flagSet.Output(), "Por defecto extrae TODO, pero ClamAV no analiza video/.")
		flagSet.PrintDefaults()
	}
	if err := flagSet.Parse(arguments); err != nil {
		return configuration, err
	}
	if configuration.ScanVideo && configuration.SkipVideoExtraction {
		return configuration, fmt.Errorf("-scan-video y -no-video-extraction no pueden usarse juntos")
	}
	if configuration.ScanVideo && configuration.SkipScan {
		return configuration, fmt.Errorf("-scan-video y -no-scan no pueden usarse juntos")
	}
	var err error
	configuration.SourceDirectory, err = filepath.Abs(configuration.SourceDirectory)
	if err != nil {
		return configuration, fmt.Errorf("resolver directorio de origen: %w", err)
	}
	configuration.DestinationDirectory, err = filepath.Abs(configuration.DestinationDirectory)
	if err != nil {
		return configuration, fmt.Errorf("resolver directorio de destino: %w", err)
	}
	configuration.SourceMKVPaths = flagSet.Args()
	configuration.OperationID = fmt.Sprintf("%s-%d", time.Now().UTC().Format("20060102T150405.000000000Z"), os.Getpid())
	return configuration, nil
}

func ValidateDependencies(configuration Configuration) error {
	required := []string{"mkvmerge", "mkvextract"}
	if !configuration.SkipScan {
		required = append(required, "clamscan")
	}
	missing := make([]string, 0)
	for _, commandName := range required {
		if _, err := exec.LookPath(commandName); err != nil {
			missing = append(missing, commandName)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("faltan herramientas: %s\nInstalacion Ubuntu:\n  sudo apt update\n  sudo apt install -y mkvtoolnix clamav clamav-freshclam\nActualizar firmas (si es necesario):\n  sudo systemctl stop clamav-freshclam\n  sudo freshclam\n  sudo systemctl start clamav-freshclam\nGo reemplaza jq: no es necesario instalarlo", strings.Join(missing, ", "))
}

func FindSourceMKVPaths(configuration Configuration) ([]string, error) {
	paths := make([]string, 0)
	if len(configuration.SourceMKVPaths) > 0 {
		for _, source := range configuration.SourceMKVPaths {
			absolutePath, err := filepath.Abs(source)
			if err != nil {
				return nil, fmt.Errorf("ruta de origen %q: %w", source, err)
			}
			paths = append(paths, absolutePath)
		}
	} else {
		entries, err := os.ReadDir(configuration.SourceDirectory)
		if err != nil {
			return nil, fmt.Errorf("leer directorio de origen %q: %w", configuration.SourceDirectory, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".mkv") {
				continue
			}
			paths = append(paths, filepath.Join(configuration.SourceDirectory, entry.Name()))
		}
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no se encontraron archivos MKV en %q", configuration.SourceDirectory)
	}
	seenPaths := make(map[string]bool)
	for _, path := range paths {
		if !strings.EqualFold(filepath.Ext(path), ".mkv") {
			return nil, fmt.Errorf("la fuente no tiene extension .mkv: %q", path)
		}
		fileInfo, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("acceder a la fuente %q: %w", path, err)
		}
		if !fileInfo.Mode().IsRegular() {
			return nil, fmt.Errorf("la fuente no es un archivo regular: %q", path)
		}
		resolvedPath, err := filepath.EvalSymlinks(path)
		if err != nil {
			return nil, fmt.Errorf("resolver la fuente %q: %w", path, err)
		}
		if seenPaths[resolvedPath] {
			return nil, fmt.Errorf("archivo MKV repetido: %q", path)
		}
		seenPaths[resolvedPath] = true
	}
	return paths, nil
}

func PrepareOperations(configuration Configuration, sourceMKVPaths []string) ([]FileOperation, error) {
	if err := ValidateDestinationRoot(configuration.DestinationDirectory); err != nil {
		return nil, err
	}
	operations := make([]FileOperation, 0, len(sourceMKVPaths))
	seenDestinationNames := make(map[string]bool)
	for _, sourceMKVPath := range sourceMKVPaths {
		sourceName := filepath.Base(sourceMKVPath)
		sourceStem := strings.TrimSuffix(sourceName, filepath.Ext(sourceName))
		if sourceStem == "" || sourceStem == "." || sourceStem == ".." {
			return nil, fmt.Errorf("nombre de MKV invalido: %q", sourceMKVPath)
		}
		if seenDestinationNames[strings.ToLower(sourceStem)] {
			return nil, fmt.Errorf("dos fuentes producirian el mismo destino: %q", sourceStem)
		}
		seenDestinationNames[strings.ToLower(sourceStem)] = true
		destinationDirectory := filepath.Join(configuration.DestinationDirectory, sourceStem)
		if _, err := os.Lstat(destinationDirectory); err == nil {
			return nil, fmt.Errorf("el destino ya existe, no se sobrescribe: %q", destinationDirectory)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("validar destino %q: %w", destinationDirectory, err)
		}

		identificationJSON, err := ReadMKVIdentificationJSON(sourceMKVPath)
		if err != nil {
			return nil, err
		}
		operation, err := BuildFileOperation(sourceMKVPath, destinationDirectory, identificationJSON, configuration.SkipVideoExtraction)
		if err != nil {
			return nil, err
		}
		operations = append(operations, operation)
	}
	return operations, nil
}

func ValidateDestinationRoot(destinationDirectory string) error {
	if fileInfo, err := os.Lstat(destinationDirectory); err == nil {
		if fileInfo.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("el directorio de destino es un enlace simbolico: %q", destinationDirectory)
		}
		if !fileInfo.IsDir() {
			return fmt.Errorf("el destino no es un directorio: %q", destinationDirectory)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("comprobar destino: %w", err)
	}
	return nil
}

func ReadMKVIdentificationJSON(sourceMKVPath string) ([]byte, error) {
	command := exec.Command("mkvmerge", "-J", sourceMKVPath)
	data, err := command.Output()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("mkvmerge no pudo identificar %q: %w: %s", sourceMKVPath, err, strings.TrimSpace(string(exitError.Stderr)))
		}
		return nil, fmt.Errorf("mkvmerge fallo para %q: %w", sourceMKVPath, err)
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("mkvmerge devolvio JSON invalido para %q", sourceMKVPath)
	}
	return data, nil
}

func BuildFileOperation(sourceMKVPath, destinationDirectory string, identificationJSON []byte, skipVideoExtraction bool) (FileOperation, error) {
	operation := FileOperation{
		SourceMKVPath:        sourceMKVPath,
		DestinationDirectory: destinationDirectory,
		IdentificationJSON:   identificationJSON,
	}
	var identification MKVIdentification
	if err := json.Unmarshal(identificationJSON, &identification); err != nil {
		return operation, fmt.Errorf("leer identificacion de %q: %w", sourceMKVPath, err)
	}
	seenTrackIDs := make(map[int]bool)
	for _, track := range identification.Tracks {
		if track.ID < 0 || seenTrackIDs[track.ID] {
			return operation, fmt.Errorf("ID de pista invalido o duplicado %d en %q", track.ID, sourceMKVPath)
		}
		seenTrackIDs[track.ID] = true
		operation.TimestampTrackIDs = append(operation.TimestampTrackIDs, track.ID)
		folder := ""
		switch track.Type {
		case "video":
			if skipVideoExtraction {
				continue
			}
			folder = "video"
		case "audio":
			folder = "audio"
		case "subtitles":
			folder = "subtitles"
		default:
			folder = "other"
		}
		extension, useRaw := ExtensionForCodec(track.Properties.CodecID)
		trackPath := filepath.Join(destinationDirectory, folder, "track-"+strconv.Itoa(track.ID)+"."+extension)
		operation.Tracks = append(operation.Tracks, TrackExtraction{
			ID: track.ID, Type: track.Type, CodecID: track.Properties.CodecID,
			DestinationFilePath: trackPath, UseRawExtraction: useRaw,
		})
	}
	seenAttachmentIDs := make(map[int]bool)
	for _, attachment := range identification.Attachments {
		if attachment.ID < 0 || seenAttachmentIDs[attachment.ID] {
			return operation, fmt.Errorf("ID de adjunto invalido o duplicado %d en %q", attachment.ID, sourceMKVPath)
		}
		seenAttachmentIDs[attachment.ID] = true
		attachmentName := SanitizeAttachmentName(attachment.FileName)
		destinationPath := filepath.Join(destinationDirectory, "attachments", strconv.Itoa(attachment.ID)+"-"+attachmentName)
		operation.Attachments = append(operation.Attachments, AttachmentExtraction{ID: attachment.ID, DestinationFilePath: destinationPath})
	}
	return operation, nil
}

func ExtensionForCodec(codecID string) (string, bool) {
	switch {
	case codecID == "V_MPEG4/ISO/AVC":
		return "h264", false
	case codecID == "V_MPEGH/ISO/HEVC":
		return "h265", false
	case codecID == "V_MPEG1", codecID == "V_MPEG2":
		return "m2v", false
	case codecID == "V_VP8", codecID == "V_VP9":
		return "ivf", false
	case codecID == "V_AV1":
		return "av1", false
	case codecID == "A_AAC" || strings.HasPrefix(codecID, "A_AAC/"):
		return "aac", false
	case codecID == "A_AC3":
		return "ac3", false
	case codecID == "A_EAC3":
		return "eac3", false
	case strings.HasPrefix(codecID, "A_DTS"):
		return "dts", false
	case codecID == "A_TRUEHD":
		return "thd", false
	case codecID == "A_FLAC":
		return "flac", false
	case codecID == "A_MPEG/L3":
		return "mp3", false
	case codecID == "A_OPUS":
		return "opus", false
	case codecID == "A_VORBIS":
		return "ogg", false
	case codecID == "S_TEXT/ASS":
		return "ass", false
	case codecID == "S_TEXT/SSA":
		return "ssa", false
	case codecID == "S_TEXT/UTF8":
		return "srt", false
	case codecID == "S_TEXT/WEBVTT":
		return "vtt", false
	case codecID == "S_HDMV/PGS":
		return "sup", false
	case codecID == "S_VOBSUB":
		return "sub", false
	default:
		// --raw conserva datos de codecs no reconocidos sin adivinar formato.
		return "bin", true
	}
}

func SanitizeAttachmentName(originalName string) string {
	// Las barras de ambos sistemas se tratan como separadores, sin rutas.
	originalName = strings.ReplaceAll(originalName, "\\", "/")
	originalName = filepath.Base(originalName)
	var name strings.Builder
	for _, character := range originalName {
		if unicode.IsControl(character) || character == '/' || character == '\\' || character == ':' {
			name.WriteRune('_')
		} else {
			name.WriteRune(character)
		}
	}
	cleanName := strings.TrimSpace(name.String())
	if cleanName == "" || cleanName == "." || cleanName == ".." {
		return "attachment.bin"
	}
	return cleanName
}

func CreateOperationLog(configuration Configuration, operations []FileOperation) (OperationLogger, error) {
	logger := OperationLogger{}
	if err := os.MkdirAll(configuration.DestinationDirectory, 0755); err != nil {
		return logger, fmt.Errorf("crear raiz de destino: %w", err)
	}
	logDirectory := filepath.Join(configuration.DestinationDirectory, "_logs")
	if err := os.MkdirAll(logDirectory, 0755); err != nil {
		return logger, fmt.Errorf("crear directorio de logs: %w", err)
	}
	logPath := filepath.Join(logDirectory, configuration.OperationID+".log")
	logFile, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return logger, fmt.Errorf("crear log %q: %w", logPath, err)
	}
	logger.File = logFile
	logger.Log(fmt.Sprintf("OPERATION START id=%s destination=%q scan=%t scanVideo=%t skipVideoExtraction=%t", configuration.OperationID, configuration.DestinationDirectory, !configuration.SkipScan, configuration.ScanVideo, configuration.SkipVideoExtraction))
	for _, operation := range operations {
		logger.Log(fmt.Sprintf("PLAN source=%q destination=%q tracks=%d attachments=%d timestamps=%d", operation.SourceMKVPath, operation.DestinationDirectory, len(operation.Tracks), len(operation.Attachments), len(operation.TimestampTrackIDs)))
	}
	fmt.Println("Log:", logPath)
	return logger, nil
}

func (logger OperationLogger) Log(message string) {
	line := time.Now().UTC().Format(time.RFC3339) + " " + message + "\n"
	fmt.Print(line)
	if logger.File != nil {
		_, _ = io.WriteString(logger.File, line)
		_ = logger.File.Sync()
	}
}

func ExtractFromSourceMKVToDestination(operation FileOperation, logger OperationLogger) error {
	// Mkdir (no MkdirAll) falla si otro proceso creo el destino tras validar.
	if err := os.Mkdir(operation.DestinationDirectory, 0755); err != nil {
		return fmt.Errorf("crear destino exclusivo %q: %w", operation.DestinationDirectory, err)
	}
	for _, directoryName := range []string{"video", "audio", "subtitles", "other", "attachments", "metadata"} {
		if err := os.Mkdir(filepath.Join(operation.DestinationDirectory, directoryName), 0755); err != nil {
			return fmt.Errorf("crear subdirectorio %q: %w", directoryName, err)
		}
	}
	identificationPath := filepath.Join(operation.DestinationDirectory, "metadata", "info.json")
	if err := WriteExclusiveFile(identificationPath, operation.IdentificationJSON); err != nil {
		return err
	}
	if err := ExtractTracksFromSourceMKV(operation, logger); err != nil {
		return err
	}
	if err := ExtractAttachmentsFromSourceMKV(operation, logger); err != nil {
		return err
	}
	if err := ExtractMetadataFromSourceMKV(operation, logger); err != nil {
		return err
	}
	logger.Log(fmt.Sprintf("EXTRACTION DONE source=%q destination=%q", operation.SourceMKVPath, operation.DestinationDirectory))
	return nil
}

func WriteExclusiveFile(destinationPath string, data []byte) error {
	file, err := os.OpenFile(destinationPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return fmt.Errorf("crear archivo sin sobrescribir %q: %w", destinationPath, err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("escribir %q: %w", destinationPath, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("cerrar %q: %w", destinationPath, err)
	}
	return nil
}

func ExtractTracksFromSourceMKV(operation FileOperation, logger OperationLogger) error {
	if len(operation.Tracks) == 0 {
		return nil
	}
	arguments := []string{operation.SourceMKVPath, "tracks"}
	for _, track := range operation.Tracks {
		if track.UseRawExtraction {
			arguments = append(arguments, "--raw")
		}
		arguments = append(arguments, strconv.Itoa(track.ID)+":"+track.DestinationFilePath)
	}
	return RunExternalCommand(logger, "mkvextract", arguments)
}

func ExtractAttachmentsFromSourceMKV(operation FileOperation, logger OperationLogger) error {
	if len(operation.Attachments) == 0 {
		return nil
	}
	arguments := []string{operation.SourceMKVPath, "attachments"}
	for _, attachment := range operation.Attachments {
		arguments = append(arguments, strconv.Itoa(attachment.ID)+":"+attachment.DestinationFilePath)
	}
	return RunExternalCommand(logger, "mkvextract", arguments)
}

func ExtractMetadataFromSourceMKV(operation FileOperation, logger OperationLogger) error {
	metadataDirectory := filepath.Join(operation.DestinationDirectory, "metadata")
	for _, metadata := range []struct{ Mode, FileName string }{
		{Mode: "chapters", FileName: "chapters.xml"},
		{Mode: "tags", FileName: "tags.xml"},
	} {
		arguments := []string{operation.SourceMKVPath, metadata.Mode, filepath.Join(metadataDirectory, metadata.FileName)}
		if err := RunExternalCommand(logger, "mkvextract", arguments); err != nil {
			return err
		}
	}
	if len(operation.TimestampTrackIDs) == 0 {
		return nil
	}
	arguments := []string{operation.SourceMKVPath, "timestamps_v2"}
	for _, trackID := range operation.TimestampTrackIDs {
		timestampPath := filepath.Join(metadataDirectory, "timestamps-"+strconv.Itoa(trackID)+".txt")
		arguments = append(arguments, strconv.Itoa(trackID)+":"+timestampPath)
	}
	return RunExternalCommand(logger, "mkvextract", arguments)
}

func ScanExtractedFilesWithClamAV(operation FileOperation, scanVideo bool, logger OperationLogger) error {
	folderNames := []string{"audio", "subtitles", "other", "attachments", "metadata"}
	if scanVideo {
		folderNames = append(folderNames, "video")
	}
	arguments := []string{
		"-r", "-i", "--alert-exceeds-max=yes",
		"--max-filesize=" + MaxClamFileSize,
		"--max-scansize=" + MaxClamScanSize,
	}
	for _, folderName := range folderNames {
		arguments = append(arguments, filepath.Join(operation.DestinationDirectory, folderName))
	}
	logger.Log(fmt.Sprintf("CLAMAV SCAN source=%q scanVideo=%t", operation.SourceMKVPath, scanVideo))
	err := RunExternalCommand(logger, "clamscan", arguments)
	if err == nil {
		logger.Log(fmt.Sprintf("CLAMAV OK source=%q (sin detecciones reportadas)", operation.SourceMKVPath))
		return nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		if exitError.ExitCode() == 1 {
			return fmt.Errorf("ClamAV reporto una deteccion o limite excedido para %q; revisar el log (codigo 1)", operation.SourceMKVPath)
		}
		if exitError.ExitCode() == 2 {
			return fmt.Errorf("ClamAV tuvo un error de analisis para %q; comprobar firmas con 'sudo freshclam' (codigo 2)", operation.SourceMKVPath)
		}
	}
	return fmt.Errorf("ClamAV fallo para %q: %w", operation.SourceMKVPath, err)
}

func RunExternalCommand(logger OperationLogger, commandName string, arguments []string) error {
	logger.Log(fmt.Sprintf("RUN %s %s", commandName, FormatArguments(arguments)))
	command := exec.Command(commandName, arguments...)
	if logger.File != nil {
		command.Stdout = io.MultiWriter(os.Stdout, logger.File)
		command.Stderr = io.MultiWriter(os.Stderr, logger.File)
	} else {
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
	}
	if err := command.Run(); err != nil {
		return fmt.Errorf("ejecutar %s: %w", commandName, err)
	}
	return nil
}

func FormatArguments(arguments []string) string {
	var buffer bytes.Buffer
	for index, argument := range arguments {
		if index > 0 {
			buffer.WriteByte(' ')
		}
		buffer.WriteString(strconv.Quote(argument))
	}
	return buffer.String()
}
