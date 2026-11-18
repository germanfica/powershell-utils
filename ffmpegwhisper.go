
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func main() {
	model := flag.String("model", "large-v3", "Whisper model")
	language := flag.String("language", "Japanese", "Audio language")
	format := flag.String("format", "txt", "Output format: txt, srt, vtt, tsv, json, all")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <video.mp4|video.mkv>\n\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "Generate FFmpeg and Whisper commands.")
		fmt.Fprintln(os.Stderr, "\nOptions:")
		flag.PrintDefaults()
	}

	flag.Parse()

	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}

	input := strings.TrimSpace(strings.Join(flag.Args(), " "))
	input = strings.Trim(input, "\"'")

	ext := strings.ToLower(filepath.Ext(input))
	if ext != ".mp4" && ext != ".mkv" {
		fmt.Fprintln(os.Stderr, "Error: expected an MP4 or MKV file.")
		os.Exit(1)
	}

	validFormats := map[string]bool{
		"txt": true,
		"srt": true,
		"vtt": true,
		"tsv": true,
		"json": true,
		"all": true,
	}

	if !validFormats[*format] {
		fmt.Fprintln(os.Stderr, "Error: invalid output format.")
		os.Exit(1)
	}

	var err error
	input, err = filepath.Abs(input)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	dir := filepath.Dir(input)
	base := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
	wav := filepath.Join(dir, base+".wav")

	fmt.Println("# 1. Extract audio")
	fmt.Printf(
		"ffmpeg -i %s \\\n"+
			"  -vn -ac 1 -ar 16000 -c:a pcm_s16le \\\n"+
			"  %s\n\n",
		shellQuote(input),
		shellQuote(wav),
	)

	fmt.Println("# 2. Transcribe audio")
	fmt.Printf(
		"uv run whisper %s \\\n"+
			"  --model %s \\\n"+
			"  --language %s \\\n"+
			"  --task transcribe \\\n"+
			"  --output_format %s \\\n"+
			"  --output_dir %s\n",
		shellQuote(wav),
		shellQuote(*model),
		shellQuote(*language),
		shellQuote(*format),
		shellQuote(dir),
	)
}
