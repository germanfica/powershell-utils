package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultDestinationFileName = "output.txt"
	DefaultLanguageMode        = "auto"
	MarkdownFence              = "```"
)

type Configuration struct {
	WorkingDirectory             string
	DestinationPath              string
	ExcludedDirectoryNames       map[string]bool
	TargetExtensions             map[string]bool
	SkippedFileNameFragments     []string
	SourcePaths                  []string
	PickFromWorkingDirectory     bool
	ShowRelativeSourcePaths      bool
	LanguageMode                 string
	DryRun                       bool
	OverwriteExistingDestination bool
	CreateDestinationSnapshot    bool
	ShowHelp                     bool
}

type PickCandidate struct {
	CandidateType string
	Name          string
	Path          string
}

type CombineOperation struct {
	OperationID                  string
	WorkingDirectory             string
	SourceFilePaths              []string
	DestinationPath              string
	TemporaryDestinationPath     string
	DestinationSnapshotPath      string
	OperationLogPath             string
	DestinationExists            bool
	DryRun                       bool
	OverwriteExistingDestination bool
	CreateDestinationSnapshot    bool
	ShowRelativeSourcePaths      bool
	LanguageMode                 string
}

type DestinationSnapshot struct {
	OperationID         string
	DestinationPath     string
	SnapshotPath        string
	DestinationFileMode os.FileMode
	BytesCopied         int64
}

func main() {
	exitCode := Run(os.Args, os.Stdin, os.Stdout, os.Stderr)
	os.Exit(exitCode)
}

func Run(arguments []string, standardInput io.Reader, standardOutput io.Writer, standardError io.Writer) int {
	workingDirectory, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(standardError, "error: cannot determine the working directory: %v\n", err)
		return 1
	}

	configuration, err := ParseCommandLineArguments(arguments, workingDirectory)
	if err != nil {
		fmt.Fprintf(standardError, "error: %v\n\n", err)
		PrintHelp(standardError)
		return 2
	}

	if configuration.ShowHelp {
		PrintHelp(standardOutput)
		return 0
	}

	if err := ValidateConfiguration(configuration); err != nil {
		fmt.Fprintf(standardError, "error: invalid configuration: %v\n", err)
		return 2
	}

	sourceRoots, err := ResolveSourceRoots(configuration, standardInput, standardOutput)
	if err != nil {
		fmt.Fprintf(standardError, "error: cannot resolve source paths: %v\n", err)
		return 1
	}

	sourceFilePaths, err := CollectSourceFiles(sourceRoots, configuration)
	if err != nil {
		fmt.Fprintf(standardError, "error: cannot collect source files: %v\n", err)
		return 1
	}

	operation, err := CreateCombineOperation(configuration, sourceFilePaths)
	if err != nil {
		fmt.Fprintf(standardError, "error: cannot create operation plan: %v\n", err)
		return 1
	}

	if err := ValidateCombineOperation(operation); err != nil {
		fmt.Fprintf(standardError, "error: operation validation failed: %v\n", err)
		return 1
	}

	if operation.DryRun {
		PrintDryRunPlan(standardOutput, operation)
		return 0
	}

	if err := ExecuteCombineOperation(operation); err != nil {
		fmt.Fprintf(standardError, "error: combine operation %s failed: %v\n", operation.OperationID, err)
		return 1
	}

	fmt.Fprintf(standardOutput, "Listo. Archivos combinados: %d\n", len(operation.SourceFilePaths))
	fmt.Fprintf(standardOutput, "Output: %s\n", operation.DestinationPath)
	fmt.Fprintf(standardOutput, "Operation ID: %s\n", operation.OperationID)
	fmt.Fprintf(standardOutput, "Log: %s\n", operation.OperationLogPath)
	if operation.DestinationExists && operation.CreateDestinationSnapshot {
		fmt.Fprintf(standardOutput, "Snapshot anterior: %s\n", operation.DestinationSnapshotPath)
	}

	return 0
}

func NewDefaultConfiguration(workingDirectory string) Configuration {
	excludedDirectoryNames := make(map[string]bool)
	excludedDirectoryNames["node_modules"] = true
	excludedDirectoryNames["dist"] = true

	targetExtensions := make(map[string]bool)
	targetExtensions[".js"] = true

	return Configuration{
		WorkingDirectory:             workingDirectory,
		DestinationPath:              filepath.Join(workingDirectory, DefaultDestinationFileName),
		ExcludedDirectoryNames:       excludedDirectoryNames,
		TargetExtensions:             targetExtensions,
		SkippedFileNameFragments:     []string{".spec.", ".test."},
		SourcePaths:                  nil,
		PickFromWorkingDirectory:     false,
		ShowRelativeSourcePaths:      false,
		LanguageMode:                 DefaultLanguageMode,
		DryRun:                       false,
		OverwriteExistingDestination: false,
		CreateDestinationSnapshot:    true,
		ShowHelp:                     false,
	}
}

func ParseCommandLineArguments(arguments []string, workingDirectory string) (Configuration, error) {
	configuration := NewDefaultConfiguration(workingDirectory)

	if len(arguments) == 0 {
		return configuration, errors.New("the program argument list is empty")
	}

	for argumentIndex := 1; argumentIndex < len(arguments); argumentIndex++ {
		argument := arguments[argumentIndex]

		if IsHelpArgument(argument) {
			configuration.ShowHelp = true
			continue
		}

		switch argument {
		case "-o", "--out":
			value, nextIndex, err := RequireOptionValue(arguments, argumentIndex, argument)
			if err != nil {
				return configuration, err
			}
			configuration.DestinationPath = ResolvePathFromWorkingDirectory(workingDirectory, value)
			argumentIndex = nextIndex

		case "--exclude":
			value, nextIndex, err := RequireOptionValue(arguments, argumentIndex, argument)
			if err != nil {
				return configuration, err
			}
			AddCommaSeparatedValuesToSet(value, configuration.ExcludedDirectoryNames, false)
			argumentIndex = nextIndex

		case "--ext":
			value, nextIndex, err := RequireOptionValue(arguments, argumentIndex, argument)
			if err != nil {
				return configuration, err
			}

			targetExtensions := make(map[string]bool)
			AddCommaSeparatedValuesToSet(value, targetExtensions, true)
			if len(targetExtensions) == 0 {
				return configuration, errors.New("--ext requires at least one non-empty extension")
			}
			configuration.TargetExtensions = targetExtensions
			argumentIndex = nextIndex

		case "--skip-name":
			value, nextIndex, err := RequireOptionValue(arguments, argumentIndex, argument)
			if err != nil {
				return configuration, err
			}
			fragments := ParseCommaSeparatedValues(value)
			if len(fragments) == 0 {
				return configuration, errors.New("--skip-name requires at least one non-empty fragment")
			}
			configuration.SkippedFileNameFragments = fragments
			argumentIndex = nextIndex

		case "--lang":
			value, nextIndex, err := RequireOptionValue(arguments, argumentIndex, argument)
			if err != nil {
				return configuration, err
			}
			configuration.LanguageMode = strings.TrimSpace(value)
			argumentIndex = nextIndex

		case "--pick":
			configuration.PickFromWorkingDirectory = true

		case "--relative":
			configuration.ShowRelativeSourcePaths = true

		case "--dry-run":
			configuration.DryRun = true

		case "--overwrite":
			configuration.OverwriteExistingDestination = true

		case "--no-snapshot":
			configuration.CreateDestinationSnapshot = false

		default:
			if strings.HasPrefix(argument, "-") {
				return configuration, fmt.Errorf("unknown option %q", argument)
			}
			configuration.SourcePaths = append(configuration.SourcePaths, ResolvePathFromWorkingDirectory(workingDirectory, argument))
		}
	}

	return configuration, nil
}

func IsHelpArgument(argument string) bool {
	return argument == "help" || argument == "-help" || argument == "-h" || argument == "--help"
}

func RequireOptionValue(arguments []string, optionIndex int, optionName string) (string, int, error) {
	valueIndex := optionIndex + 1
	if valueIndex >= len(arguments) {
		return "", optionIndex, fmt.Errorf("%s requires a value", optionName)
	}

	value := strings.TrimSpace(arguments[valueIndex])
	if value == "" {
		return "", optionIndex, fmt.Errorf("%s requires a non-empty value", optionName)
	}

	return value, valueIndex, nil
}

func ResolvePathFromWorkingDirectory(workingDirectory string, configuredPath string) string {
	if filepath.IsAbs(configuredPath) {
		return filepath.Clean(configuredPath)
	}
	return filepath.Clean(filepath.Join(workingDirectory, configuredPath))
}

func AddCommaSeparatedValuesToSet(rawValues string, destinationSet map[string]bool, normalizeAsExtension bool) {
	values := ParseCommaSeparatedValues(rawValues)
	for _, value := range values {
		if normalizeAsExtension {
			value = NormalizeExtension(value)
		}
		if value != "" {
			destinationSet[value] = true
		}
	}
}

func ParseCommaSeparatedValues(rawValues string) []string {
	parts := strings.Split(rawValues, ",")
	values := make([]string, 0, len(parts))

	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		values = append(values, value)
	}

	return values
}

func NormalizeExtension(extension string) string {
	normalizedExtension := strings.ToLower(strings.TrimSpace(extension))
	if normalizedExtension == "" {
		return ""
	}
	if strings.HasPrefix(normalizedExtension, ".") {
		return normalizedExtension
	}
	return "." + normalizedExtension
}

func ValidateConfiguration(configuration Configuration) error {
	if configuration.WorkingDirectory == "" {
		return errors.New("working directory is empty")
	}

	workingDirectoryInfo, err := os.Stat(configuration.WorkingDirectory)
	if err != nil {
		return fmt.Errorf("cannot access working directory %q: %w", configuration.WorkingDirectory, err)
	}
	if !workingDirectoryInfo.IsDir() {
		return fmt.Errorf("working directory %q is not a directory", configuration.WorkingDirectory)
	}

	if configuration.DestinationPath == "" {
		return errors.New("destination path is empty")
	}

	if len(configuration.TargetExtensions) == 0 {
		return errors.New("no target extensions are configured")
	}

	for extension := range configuration.TargetExtensions {
		if NormalizeExtension(extension) != extension {
			return fmt.Errorf("target extension %q is not normalized", extension)
		}
	}

	if strings.TrimSpace(configuration.LanguageMode) == "" {
		return errors.New("language mode is empty")
	}

	if configuration.PickFromWorkingDirectory && len(configuration.SourcePaths) > 0 {
		return errors.New("--pick cannot be combined with explicit source paths")
	}

	return nil
}

func ResolveSourceRoots(configuration Configuration, standardInput io.Reader, standardOutput io.Writer) ([]string, error) {
	if len(configuration.SourcePaths) > 0 {
		return CopyStrings(configuration.SourcePaths), nil
	}

	if !configuration.PickFromWorkingDirectory {
		return []string{configuration.WorkingDirectory}, nil
	}

	pickCandidates, err := ListPickCandidates(configuration.WorkingDirectory, configuration)
	if err != nil {
		return nil, err
	}
	if len(pickCandidates) == 0 {
		return nil, errors.New("no candidate directories or files were found in the working directory")
	}

	fmt.Fprintln(standardOutput, "")
	fmt.Fprintln(standardOutput, "Elegí qué incluir (items del directorio actual):")
	for candidateIndex, candidate := range pickCandidates {
		fmt.Fprintf(standardOutput, "  %d. [%s] %s\n", candidateIndex+1, candidate.CandidateType, candidate.Name)
	}
	fmt.Fprintln(standardOutput, "")
	fmt.Fprint(standardOutput, "Ingresá indices (ej: 1,3,5-7) o Enter para TODO: ")

	reader := bufio.NewReader(standardInput)
	selectionText, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("cannot read interactive selection: %w", err)
	}

	selectedIndexes, selectAll, err := ParsePickSelection(selectionText, len(pickCandidates))
	if err != nil {
		return nil, err
	}

	if selectAll {
		return []string{configuration.WorkingDirectory}, nil
	}

	selectedPaths := make([]string, 0, len(selectedIndexes))
	for _, selectedIndex := range selectedIndexes {
		selectedPaths = append(selectedPaths, pickCandidates[selectedIndex-1].Path)
	}
	return selectedPaths, nil
}

func ListPickCandidates(baseDirectory string, configuration Configuration) ([]PickCandidate, error) {
	entries, err := os.ReadDir(baseDirectory)
	if err != nil {
		return nil, fmt.Errorf("cannot read directory %q: %w", baseDirectory, err)
	}

	candidates := make([]PickCandidate, 0)
	for _, entry := range entries {
		candidatePath := filepath.Join(baseDirectory, entry.Name())

		entryInfo, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("cannot inspect candidate %q: %w", candidatePath, err)
		}
		if entryInfo.Mode()&os.ModeSymlink != 0 {
			continue
		}

		if entryInfo.IsDir() {
			if configuration.ExcludedDirectoryNames[entry.Name()] {
				continue
			}
			candidates = append(candidates, PickCandidate{
				CandidateType: "dir",
				Name:          entry.Name(),
				Path:          candidatePath,
			})
			continue
		}

		if !entryInfo.Mode().IsRegular() {
			continue
		}
		if !HasTargetExtension(candidatePath, configuration.TargetExtensions) {
			continue
		}
		if IsSkippedByFileName(candidatePath, configuration.SkippedFileNameFragments) {
			continue
		}

		candidates = append(candidates, PickCandidate{
			CandidateType: "file",
			Name:          entry.Name(),
			Path:          candidatePath,
		})
	}

	sort.Slice(candidates, func(leftIndex int, rightIndex int) bool {
		leftCandidate := candidates[leftIndex]
		rightCandidate := candidates[rightIndex]

		if leftCandidate.CandidateType != rightCandidate.CandidateType {
			return leftCandidate.CandidateType == "dir"
		}
		return leftCandidate.Name < rightCandidate.Name
	})

	return candidates, nil
}

func ParsePickSelection(selectionText string, maximumIndex int) ([]int, bool, error) {
	trimmedSelection := strings.TrimSpace(selectionText)
	if trimmedSelection == "" {
		return nil, true, nil
	}
	if maximumIndex < 1 {
		return nil, false, errors.New("maximum selection index must be at least 1")
	}

	chosenIndexes := make(map[int]bool)
	selectionParts := strings.Split(trimmedSelection, ",")

	for _, rawSelectionPart := range selectionParts {
		selectionPart := strings.TrimSpace(rawSelectionPart)
		if selectionPart == "" {
			return nil, false, errors.New("selection contains an empty item")
		}

		if strings.Contains(selectionPart, "-") {
			if strings.Count(selectionPart, "-") != 1 {
				return nil, false, fmt.Errorf("invalid selection range %q", selectionPart)
			}

			rangeParts := strings.Split(selectionPart, "-")
			startIndex, err := ParseSelectionIndex(rangeParts[0], maximumIndex)
			if err != nil {
				return nil, false, err
			}
			endIndex, err := ParseSelectionIndex(rangeParts[1], maximumIndex)
			if err != nil {
				return nil, false, err
			}

			if startIndex > endIndex {
				startIndex, endIndex = endIndex, startIndex
			}
			for selectedIndex := startIndex; selectedIndex <= endIndex; selectedIndex++ {
				chosenIndexes[selectedIndex] = true
			}
			continue
		}

		selectedIndex, err := ParseSelectionIndex(selectionPart, maximumIndex)
		if err != nil {
			return nil, false, err
		}
		chosenIndexes[selectedIndex] = true
	}

	selectedIndexes := make([]int, 0, len(chosenIndexes))
	for selectedIndex := range chosenIndexes {
		selectedIndexes = append(selectedIndexes, selectedIndex)
	}
	sort.Ints(selectedIndexes)

	if len(selectedIndexes) == 0 {
		return nil, false, errors.New("selection did not contain valid indexes")
	}

	return selectedIndexes, false, nil
}

func ParseSelectionIndex(rawIndex string, maximumIndex int) (int, error) {
	trimmedIndex := strings.TrimSpace(rawIndex)
	selectedIndex, err := strconv.Atoi(trimmedIndex)
	if err != nil {
		return 0, fmt.Errorf("invalid selection index %q", trimmedIndex)
	}
	if selectedIndex < 1 || selectedIndex > maximumIndex {
		return 0, fmt.Errorf("selection index %d is outside the valid range 1-%d", selectedIndex, maximumIndex)
	}
	return selectedIndex, nil
}

func CollectSourceFiles(sourceRoots []string, configuration Configuration) ([]string, error) {
	if len(sourceRoots) == 0 {
		return nil, errors.New("no source roots were provided")
	}

	collectedSourceFilePaths := make([]string, 0)
	for _, sourceRoot := range sourceRoots {
		sourceFilePaths, err := WalkForTargetFiles(sourceRoot, configuration)
		if err != nil {
			return nil, err
		}
		collectedSourceFilePaths = append(collectedSourceFilePaths, sourceFilePaths...)
	}

	return UniqueSortedPaths(collectedSourceFilePaths), nil
}

func WalkForTargetFiles(sourceRoot string, configuration Configuration) ([]string, error) {
	sourceRootInfo, err := os.Stat(sourceRoot)
	if err != nil {
		return nil, fmt.Errorf("cannot access source path %q: %w", sourceRoot, err)
	}

	cleanSourceRoot := filepath.Clean(sourceRoot)
	if IsExcludedPath(cleanSourceRoot, configuration.ExcludedDirectoryNames) {
		return nil, nil
	}

	if sourceRootInfo.Mode().IsRegular() {
		if IsEligibleSourceFile(cleanSourceRoot, configuration) {
			return []string{cleanSourceRoot}, nil
		}
		return nil, nil
	}

	if !sourceRootInfo.IsDir() {
		return nil, fmt.Errorf("source path %q is neither a regular file nor a directory", cleanSourceRoot)
	}

	collectedSourceFilePaths := make([]string, 0)
	err = filepath.Walk(cleanSourceRoot, func(currentPath string, currentInfo os.FileInfo, walkError error) error {
		if walkError != nil {
			return fmt.Errorf("cannot inspect source path %q: %w", currentPath, walkError)
		}

		if currentInfo.Mode()&os.ModeSymlink != 0 {
			if currentInfo.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if currentInfo.IsDir() {
			if currentPath != cleanSourceRoot && IsExcludedPath(currentPath, configuration.ExcludedDirectoryNames) {
				return filepath.SkipDir
			}
			return nil
		}

		if !currentInfo.Mode().IsRegular() {
			return nil
		}
		if IsEligibleSourceFile(currentPath, configuration) {
			collectedSourceFilePaths = append(collectedSourceFilePaths, filepath.Clean(currentPath))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return collectedSourceFilePaths, nil
}

func IsEligibleSourceFile(sourceFilePath string, configuration Configuration) bool {
	if IsExcludedPath(sourceFilePath, configuration.ExcludedDirectoryNames) {
		return false
	}
	if !HasTargetExtension(sourceFilePath, configuration.TargetExtensions) {
		return false
	}
	if IsSkippedByFileName(sourceFilePath, configuration.SkippedFileNameFragments) {
		return false
	}
	return true
}

func IsExcludedPath(candidatePath string, excludedDirectoryNames map[string]bool) bool {
	cleanPath := filepath.Clean(candidatePath)
	volumeName := filepath.VolumeName(cleanPath)
	pathWithoutVolume := strings.TrimPrefix(cleanPath, volumeName)
	pathParts := strings.FieldsFunc(pathWithoutVolume, func(character rune) bool {
		return character == '/' || character == '\\'
	})

	for _, pathPart := range pathParts {
		if excludedDirectoryNames[pathPart] {
			return true
		}
	}
	return false
}

func HasTargetExtension(sourceFilePath string, targetExtensions map[string]bool) bool {
	extension := strings.ToLower(filepath.Ext(sourceFilePath))
	return targetExtensions[extension]
}

func IsSkippedByFileName(sourceFilePath string, skippedFileNameFragments []string) bool {
	baseName := strings.ToLower(filepath.Base(sourceFilePath))
	for _, skippedFragment := range skippedFileNameFragments {
		normalizedFragment := strings.ToLower(strings.TrimSpace(skippedFragment))
		if normalizedFragment == "" {
			continue
		}
		if strings.Contains(baseName, normalizedFragment) {
			return true
		}
	}
	return false
}

func UniqueSortedPaths(paths []string) []string {
	uniquePaths := make(map[string]bool)
	for _, candidatePath := range paths {
		uniquePaths[filepath.Clean(candidatePath)] = true
	}

	sortedPaths := make([]string, 0, len(uniquePaths))
	for uniquePath := range uniquePaths {
		sortedPaths = append(sortedPaths, uniquePath)
	}
	sort.Strings(sortedPaths)
	return sortedPaths
}

func CreateCombineOperation(configuration Configuration, sourceFilePaths []string) (CombineOperation, error) {
	operationID := CreateOperationID()
	destinationPath := filepath.Clean(configuration.DestinationPath)
	destinationDirectory := filepath.Dir(destinationPath)
	destinationBaseName := filepath.Base(destinationPath)

	destinationInfo, destinationStatError := os.Stat(destinationPath)
	destinationExists := destinationStatError == nil
	if destinationStatError != nil && !os.IsNotExist(destinationStatError) {
		return CombineOperation{}, fmt.Errorf("cannot inspect destination path %q: %w", destinationPath, destinationStatError)
	}
	if destinationExists && destinationInfo.IsDir() {
		return CombineOperation{}, fmt.Errorf("destination path %q is a directory", destinationPath)
	}

	temporaryDestinationPath := filepath.Join(destinationDirectory, "."+destinationBaseName+"."+operationID+".tmp")
	destinationSnapshotPath := destinationPath + ".snapshot." + operationID
	operationLogPath := destinationPath + ".operation." + operationID + ".log"

	return CombineOperation{
		OperationID:                  operationID,
		WorkingDirectory:             configuration.WorkingDirectory,
		SourceFilePaths:              CopyStrings(sourceFilePaths),
		DestinationPath:              destinationPath,
		TemporaryDestinationPath:     temporaryDestinationPath,
		DestinationSnapshotPath:      destinationSnapshotPath,
		OperationLogPath:             operationLogPath,
		DestinationExists:            destinationExists,
		DryRun:                       configuration.DryRun,
		OverwriteExistingDestination: configuration.OverwriteExistingDestination,
		CreateDestinationSnapshot:    configuration.CreateDestinationSnapshot,
		ShowRelativeSourcePaths:      configuration.ShowRelativeSourcePaths,
		LanguageMode:                 configuration.LanguageMode,
	}, nil
}

func CreateOperationID() string {
	timestamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	return fmt.Sprintf("%s-pid%d", timestamp, os.Getpid())
}

func ValidateCombineOperation(operation CombineOperation) error {
	if operation.OperationID == "" {
		return errors.New("operation ID is empty")
	}
	if operation.WorkingDirectory == "" {
		return errors.New("working directory is empty")
	}
	if operation.DestinationPath == "" {
		return errors.New("destination path is empty")
	}
	if operation.TemporaryDestinationPath == "" {
		return errors.New("temporary destination path is empty")
	}
	if operation.OperationLogPath == "" {
		return errors.New("operation log path is empty")
	}
	if strings.TrimSpace(operation.LanguageMode) == "" {
		return errors.New("language mode is empty")
	}

	destinationDirectory := filepath.Dir(operation.DestinationPath)
	destinationDirectoryInfo, err := os.Stat(destinationDirectory)
	if err != nil {
		return fmt.Errorf("cannot access destination directory %q: %w", destinationDirectory, err)
	}
	if !destinationDirectoryInfo.IsDir() {
		return fmt.Errorf("destination parent %q is not a directory", destinationDirectory)
	}

	if operation.DestinationExists && !operation.OverwriteExistingDestination && !operation.DryRun {
		return fmt.Errorf("destination %q already exists; use --overwrite after reviewing --dry-run", operation.DestinationPath)
	}

	if operation.DestinationExists && operation.CreateDestinationSnapshot && operation.DestinationSnapshotPath == "" {
		return errors.New("destination snapshot path is empty")
	}

	if PathsReferToSameLocation(operation.DestinationPath, operation.TemporaryDestinationPath) {
		return errors.New("destination path and temporary destination path are the same")
	}

	for _, sourceFilePath := range operation.SourceFilePaths {
		if err := ValidateSourceFileForOperation(sourceFilePath, operation); err != nil {
			return err
		}
	}

	if PathExists(operation.TemporaryDestinationPath) {
		return fmt.Errorf("temporary destination path already exists: %q", operation.TemporaryDestinationPath)
	}
	if PathExists(operation.OperationLogPath) {
		return fmt.Errorf("operation log path already exists: %q", operation.OperationLogPath)
	}
	if operation.DestinationExists && operation.CreateDestinationSnapshot && PathExists(operation.DestinationSnapshotPath) {
		return fmt.Errorf("destination snapshot path already exists: %q", operation.DestinationSnapshotPath)
	}

	return nil
}

func ValidateSourceFileForOperation(sourceFilePath string, operation CombineOperation) error {
	sourceInfo, err := os.Stat(sourceFilePath)
	if err != nil {
		return fmt.Errorf("cannot access source file %q: %w", sourceFilePath, err)
	}
	if !sourceInfo.Mode().IsRegular() {
		return fmt.Errorf("source path %q is not a regular file", sourceFilePath)
	}
	if PathsReferToSameLocation(sourceFilePath, operation.DestinationPath) {
		return fmt.Errorf("destination path %q is also present in the source file list", operation.DestinationPath)
	}
	if PathsReferToSameLocation(sourceFilePath, operation.TemporaryDestinationPath) {
		return fmt.Errorf("temporary destination path %q is also present in the source file list", operation.TemporaryDestinationPath)
	}
	return nil
}

func PathsReferToSameLocation(leftPath string, rightPath string) bool {
	leftAbsolutePath, leftError := filepath.Abs(leftPath)
	rightAbsolutePath, rightError := filepath.Abs(rightPath)
	if leftError != nil || rightError != nil {
		return filepath.Clean(leftPath) == filepath.Clean(rightPath)
	}
	return filepath.Clean(leftAbsolutePath) == filepath.Clean(rightAbsolutePath)
}

func PathExists(candidatePath string) bool {
	_, err := os.Stat(candidatePath)
	return err == nil
}

func ExecuteCombineOperation(operation CombineOperation) error {
	if err := WriteInitialOperationLog(operation); err != nil {
		return err
	}

	if operation.DestinationExists && operation.CreateDestinationSnapshot {
		snapshot, err := CreateDestinationSnapshot(operation)
		if err != nil {
			AppendOperationLogBestEffort(operation.OperationLogPath, "snapshot_failed", err.Error())
			return err
		}
		AppendOperationLogBestEffort(operation.OperationLogPath, "snapshot_created", fmt.Sprintf("path=%s bytes=%d", snapshot.SnapshotPath, snapshot.BytesCopied))
	}

	if err := WriteCombinedOutputToTemporaryDestination(operation); err != nil {
		AppendOperationLogBestEffort(operation.OperationLogPath, "temporary_write_failed", err.Error())
		RemoveTemporaryDestinationBestEffort(operation.TemporaryDestinationPath)
		return err
	}
	AppendOperationLogBestEffort(operation.OperationLogPath, "temporary_write_complete", operation.TemporaryDestinationPath)

	if err := ReplaceDestinationWithTemporaryDestination(operation); err != nil {
		AppendOperationLogBestEffort(operation.OperationLogPath, "destination_replace_failed", err.Error())
		RemoveTemporaryDestinationBestEffort(operation.TemporaryDestinationPath)
		return err
	}

	AppendOperationLogBestEffort(operation.OperationLogPath, "operation_complete", operation.DestinationPath)
	return nil
}

func WriteInitialOperationLog(operation CombineOperation) error {
	operationLogFile, err := os.OpenFile(operation.OperationLogPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("cannot create operation log %q: %w", operation.OperationLogPath, err)
	}

	writeError := WriteOperationLogContents(operationLogFile, operation)
	closeError := operationLogFile.Close()
	if writeError != nil {
		return fmt.Errorf("cannot write operation log %q: %w", operation.OperationLogPath, writeError)
	}
	if closeError != nil {
		return fmt.Errorf("cannot close operation log %q: %w", operation.OperationLogPath, closeError)
	}
	return nil
}

func WriteOperationLogContents(destination io.Writer, operation CombineOperation) error {
	lines := []string{
		"operation_id=" + operation.OperationID,
		"created_utc=" + time.Now().UTC().Format(time.RFC3339Nano),
		"working_directory=" + operation.WorkingDirectory,
		"destination_path=" + operation.DestinationPath,
		"temporary_destination_path=" + operation.TemporaryDestinationPath,
		"destination_snapshot_path=" + operation.DestinationSnapshotPath,
		"destination_exists=" + strconv.FormatBool(operation.DestinationExists),
		"overwrite_existing_destination=" + strconv.FormatBool(operation.OverwriteExistingDestination),
		"create_destination_snapshot=" + strconv.FormatBool(operation.CreateDestinationSnapshot),
		"show_relative_source_paths=" + strconv.FormatBool(operation.ShowRelativeSourcePaths),
		"language_mode=" + operation.LanguageMode,
		"source_file_count=" + strconv.Itoa(len(operation.SourceFilePaths)),
	}

	for _, line := range lines {
		if _, err := fmt.Fprintln(destination, line); err != nil {
			return err
		}
	}
	for sourceIndex, sourceFilePath := range operation.SourceFilePaths {
		if _, err := fmt.Fprintf(destination, "source_file_%d=%s\n", sourceIndex+1, sourceFilePath); err != nil {
			return err
		}
	}
	return nil
}

func AppendOperationLogBestEffort(operationLogPath string, eventName string, eventDetails string) {
	operationLogFile, err := os.OpenFile(operationLogPath, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return
	}
	defer operationLogFile.Close()

	fmt.Fprintf(operationLogFile, "event=%s time_utc=%s details=%s\n", eventName, time.Now().UTC().Format(time.RFC3339Nano), eventDetails)
}

func CreateDestinationSnapshot(operation CombineOperation) (DestinationSnapshot, error) {
	if !operation.DestinationExists {
		return DestinationSnapshot{}, errors.New("cannot create a destination snapshot because the destination does not exist")
	}
	if operation.DestinationSnapshotPath == "" {
		return DestinationSnapshot{}, errors.New("cannot create a destination snapshot because the snapshot path is empty")
	}

	destinationFile, err := os.Open(operation.DestinationPath)
	if err != nil {
		return DestinationSnapshot{}, fmt.Errorf("cannot open destination %q for snapshot: %w", operation.DestinationPath, err)
	}
	defer destinationFile.Close()

	destinationInfo, err := destinationFile.Stat()
	if err != nil {
		return DestinationSnapshot{}, fmt.Errorf("cannot inspect destination %q for snapshot: %w", operation.DestinationPath, err)
	}
	if !destinationInfo.Mode().IsRegular() {
		return DestinationSnapshot{}, fmt.Errorf("destination %q is not a regular file", operation.DestinationPath)
	}

	snapshotFile, err := os.OpenFile(operation.DestinationSnapshotPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, destinationInfo.Mode().Perm())
	if err != nil {
		return DestinationSnapshot{}, fmt.Errorf("cannot create destination snapshot %q: %w", operation.DestinationSnapshotPath, err)
	}

	bytesCopied, copyError := io.Copy(snapshotFile, destinationFile)
	syncError := snapshotFile.Sync()
	closeError := snapshotFile.Close()

	if copyError != nil {
		os.Remove(operation.DestinationSnapshotPath)
		return DestinationSnapshot{}, fmt.Errorf("cannot copy destination %q to snapshot %q: %w", operation.DestinationPath, operation.DestinationSnapshotPath, copyError)
	}
	if syncError != nil {
		os.Remove(operation.DestinationSnapshotPath)
		return DestinationSnapshot{}, fmt.Errorf("cannot sync destination snapshot %q: %w", operation.DestinationSnapshotPath, syncError)
	}
	if closeError != nil {
		os.Remove(operation.DestinationSnapshotPath)
		return DestinationSnapshot{}, fmt.Errorf("cannot close destination snapshot %q: %w", operation.DestinationSnapshotPath, closeError)
	}

	return DestinationSnapshot{
		OperationID:         operation.OperationID,
		DestinationPath:     operation.DestinationPath,
		SnapshotPath:        operation.DestinationSnapshotPath,
		DestinationFileMode: destinationInfo.Mode().Perm(),
		BytesCopied:         bytesCopied,
	}, nil
}

func RestoreDestinationFromSnapshot(operation CombineOperation) error {
	if operation.DestinationSnapshotPath == "" {
		return errors.New("snapshot path is empty")
	}

	snapshotInfo, err := os.Stat(operation.DestinationSnapshotPath)
	if err != nil {
		return fmt.Errorf("cannot access snapshot %q: %w", operation.DestinationSnapshotPath, err)
	}
	if !snapshotInfo.Mode().IsRegular() {
		return fmt.Errorf("snapshot %q is not a regular file", operation.DestinationSnapshotPath)
	}

	snapshotFile, err := os.Open(operation.DestinationSnapshotPath)
	if err != nil {
		return fmt.Errorf("cannot open snapshot %q: %w", operation.DestinationSnapshotPath, err)
	}
	defer snapshotFile.Close()

	restoredDestinationFile, err := os.OpenFile(operation.DestinationPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, snapshotInfo.Mode().Perm())
	if err != nil {
		return fmt.Errorf("cannot open destination %q for restore: %w", operation.DestinationPath, err)
	}

	_, copyError := io.Copy(restoredDestinationFile, snapshotFile)
	syncError := restoredDestinationFile.Sync()
	closeError := restoredDestinationFile.Close()

	if copyError != nil {
		return fmt.Errorf("cannot restore destination %q from snapshot %q: %w", operation.DestinationPath, operation.DestinationSnapshotPath, copyError)
	}
	if syncError != nil {
		return fmt.Errorf("cannot sync restored destination %q: %w", operation.DestinationPath, syncError)
	}
	if closeError != nil {
		return fmt.Errorf("cannot close restored destination %q: %w", operation.DestinationPath, closeError)
	}
	return nil
}

func WriteCombinedOutputToTemporaryDestination(operation CombineOperation) error {
	temporaryDestinationFile, err := os.OpenFile(operation.TemporaryDestinationPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return fmt.Errorf("cannot create temporary destination %q: %w", operation.TemporaryDestinationPath, err)
	}

	writeError := WriteCombinedOutput(temporaryDestinationFile, operation)
	syncError := temporaryDestinationFile.Sync()
	closeError := temporaryDestinationFile.Close()

	if writeError != nil {
		return fmt.Errorf("cannot write temporary destination %q: %w", operation.TemporaryDestinationPath, writeError)
	}
	if syncError != nil {
		return fmt.Errorf("cannot sync temporary destination %q: %w", operation.TemporaryDestinationPath, syncError)
	}
	if closeError != nil {
		return fmt.Errorf("cannot close temporary destination %q: %w", operation.TemporaryDestinationPath, closeError)
	}
	return nil
}

func WriteCombinedOutput(destination io.Writer, operation CombineOperation) error {
	for _, sourceFilePath := range operation.SourceFilePaths {
		displayName, err := CreateSourceDisplayName(sourceFilePath, operation.WorkingDirectory, operation.ShowRelativeSourcePaths)
		if err != nil {
			return err
		}

		fenceInformation := FenceInformationForSourceFile(sourceFilePath, operation.LanguageMode)
		fenceLine := MarkdownFence
		if fenceInformation != "" {
			fenceLine += fenceInformation
		}

		if _, err := fmt.Fprintf(destination, "archivo: %s\n%s\n", displayName, fenceLine); err != nil {
			return fmt.Errorf("cannot write header for source file %q: %w", sourceFilePath, err)
		}

		sourceFileEndedWithLineBreak, err := CopySourceFileToDestination(sourceFilePath, destination)
		if err != nil {
			return err
		}
		if !sourceFileEndedWithLineBreak {
			if _, err := io.WriteString(destination, "\n"); err != nil {
				return fmt.Errorf("cannot write line break after source file %q: %w", sourceFilePath, err)
			}
		}
		if _, err := fmt.Fprintf(destination, "%s\n", MarkdownFence); err != nil {
			return fmt.Errorf("cannot write closing fence for source file %q: %w", sourceFilePath, err)
		}
	}
	return nil
}

func CreateSourceDisplayName(sourceFilePath string, workingDirectory string, showRelativeSourcePaths bool) (string, error) {
	if !showRelativeSourcePaths {
		return filepath.Base(sourceFilePath), nil
	}

	relativePath, err := filepath.Rel(workingDirectory, sourceFilePath)
	if err != nil {
		return "", fmt.Errorf("cannot create relative path for source file %q: %w", sourceFilePath, err)
	}
	return relativePath, nil
}

func CopySourceFileToDestination(sourceFilePath string, destination io.Writer) (bool, error) {
	sourceFile, err := os.Open(sourceFilePath)
	if err != nil {
		return false, fmt.Errorf("cannot open source file %q: %w", sourceFilePath, err)
	}
	defer sourceFile.Close()

	if _, err := io.Copy(destination, sourceFile); err != nil {
		return false, fmt.Errorf("cannot copy source file %q: %w", sourceFilePath, err)
	}

	sourceInfo, err := sourceFile.Stat()
	if err != nil {
		return false, fmt.Errorf("cannot inspect source file %q after copy: %w", sourceFilePath, err)
	}
	if sourceInfo.Size() == 0 {
		return false, nil
	}

	if _, err := sourceFile.Seek(-1, io.SeekEnd); err != nil {
		return false, fmt.Errorf("cannot inspect final byte of source file %q: %w", sourceFilePath, err)
	}

	lastByte := make([]byte, 1)
	if _, err := io.ReadFull(sourceFile, lastByte); err != nil {
		return false, fmt.Errorf("cannot read final byte of source file %q: %w", sourceFilePath, err)
	}
	return lastByte[0] == '\n' || lastByte[0] == '\r', nil
}

func FenceInformationForSourceFile(sourceFilePath string, languageMode string) string {
	if languageMode == "none" {
		return ""
	}
	if languageMode != "auto" {
		return languageMode
	}

	switch strings.ToLower(filepath.Ext(sourceFilePath)) {
	case ".js":
		return "javascript"
	case ".jsx":
		return "jsx"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "tsx"
	case ".json":
		return "json"
	case ".md":
		return "markdown"
	case ".py":
		return "python"
	case ".rb":
		return "ruby"
	case ".go":
		return "go"
	case ".java":
		return "java"
	case ".kt":
		return "kotlin"
	case ".c", ".h":
		return "c"
	case ".cpp", ".hpp":
		return "cpp"
	case ".cs":
		return "csharp"
	case ".rs":
		return "rust"
	case ".php":
		return "php"
	case ".sh":
		return "bash"
	case ".zsh":
		return "zsh"
	case ".yaml", ".yml":
		return "yaml"
	case ".toml":
		return "toml"
	case ".sql":
		return "sql"
	case ".html":
		return "html"
	case ".css":
		return "css"
	case ".scss":
		return "scss"
	default:
		return ""
	}
}

func ReplaceDestinationWithTemporaryDestination(operation CombineOperation) error {
	if operation.DestinationExists {
		if !operation.OverwriteExistingDestination {
			return fmt.Errorf("refusing to replace existing destination %q without --overwrite", operation.DestinationPath)
		}

		if err := os.Remove(operation.DestinationPath); err != nil {
			return fmt.Errorf("cannot remove existing destination %q after validation and snapshot: %w", operation.DestinationPath, err)
		}
	}

	if err := os.Rename(operation.TemporaryDestinationPath, operation.DestinationPath); err != nil {
		if operation.DestinationExists && operation.CreateDestinationSnapshot {
			restoreError := RestoreDestinationFromSnapshot(operation)
			if restoreError != nil {
				return fmt.Errorf("cannot move temporary destination %q to %q: %v; restore from snapshot also failed: %w", operation.TemporaryDestinationPath, operation.DestinationPath, err, restoreError)
			}
			return fmt.Errorf("cannot move temporary destination %q to %q: %w; the previous destination was restored from %q", operation.TemporaryDestinationPath, operation.DestinationPath, err, operation.DestinationSnapshotPath)
		}
		return fmt.Errorf("cannot move temporary destination %q to %q: %w", operation.TemporaryDestinationPath, operation.DestinationPath, err)
	}
	return nil
}

func RemoveTemporaryDestinationBestEffort(temporaryDestinationPath string) {
	if temporaryDestinationPath == "" {
		return
	}
	os.Remove(temporaryDestinationPath)
}

func PrintDryRunPlan(destination io.Writer, operation CombineOperation) {
	fmt.Fprintln(destination, "DRY RUN: no se modificó ningún archivo.")
	fmt.Fprintf(destination, "Operation ID: %s\n", operation.OperationID)
	fmt.Fprintf(destination, "Archivos que se combinarían: %d\n", len(operation.SourceFilePaths))
	for _, sourceFilePath := range operation.SourceFilePaths {
		fmt.Fprintf(destination, "  %s\n", sourceFilePath)
	}
	fmt.Fprintf(destination, "Destination: %s\n", operation.DestinationPath)
	fmt.Fprintf(destination, "Destination exists: %t\n", operation.DestinationExists)
	fmt.Fprintf(destination, "Overwrite enabled: %t\n", operation.OverwriteExistingDestination)
	fmt.Fprintf(destination, "Snapshot enabled: %t\n", operation.CreateDestinationSnapshot)
	if operation.DestinationExists && !operation.OverwriteExistingDestination {
		fmt.Fprintln(destination, "Nota: la ejecución real requerirá --overwrite.")
	}
	if operation.DestinationExists && operation.CreateDestinationSnapshot {
		fmt.Fprintf(destination, "Snapshot path: %s\n", operation.DestinationSnapshotPath)
	}
	fmt.Fprintf(destination, "Operation log path: %s\n", operation.OperationLogPath)
}

func PrintHelp(destination io.Writer) {
	fmt.Fprintf(destination, `Uso:
  combine-files [rutas...] [opciones]

Si NO pasas rutas:
  - combina desde el directorio actual de forma recursiva
  - ignora node_modules/ y dist/

Si pasas rutas:
  - cada ruta puede ser una carpeta recursiva o un archivo
  - los archivos deben coincidir con --ext

Opciones:
  -o, --out <archivo>       Destination (default: %s)
  --exclude <a,b,c>         Agrega nombres de carpetas a excluir
                            (default: node_modules,dist)
  --ext <js,ts,py,...>      Extensiones a incluir (default: js)
  --skip-name <a,b,c>       Excluye archivos cuyo nombre contiene fragmentos
                            (default: .spec.,.test.)
  --lang <auto|none|X>      Fence markdown: auto detecta por extensión,
                            none no agrega lenguaje y X fuerza un lenguaje
  --relative                Escribe rutas relativas después de "archivo:"
  --pick                    Elige items del directorio actual de forma interactiva
  --dry-run                 Valida y muestra el plan sin escribir archivos
  --overwrite               Permite reemplazar un destination existente
  --no-snapshot             No crea snapshot antes de --overwrite
  help, -help, -h, --help   Muestra esta ayuda

Seguridad al escribir:
  - nunca sobrescribe sin --overwrite
  - por defecto crea un snapshot antes de sobrescribir
  - escribe primero un log de operación
  - genera el contenido en un archivo temporal y luego reemplaza el destination

Ejemplos:
  combine-files help
  combine-files
  combine-files --dry-run
  combine-files src shared/util.py --ext py
  combine-files packages/app packages/lib --ext js,ts --relative -o combined.txt
  combine-files --lang text --ext js,ts,py
  combine-files --pick --ext js,ts
  combine-files --overwrite -o output.txt
`, DefaultDestinationFileName)
}

func CopyStrings(values []string) []string {
	copiedValues := make([]string, len(values))
	copy(copiedValues, values)
	return copiedValues
}